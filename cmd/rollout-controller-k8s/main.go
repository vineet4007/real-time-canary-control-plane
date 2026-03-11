package main

import (
	"context"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	controllerrollout "github.com/vineet4007/real-time-canary-control-plane/internal/controller/rollout"
	rolloutpb "github.com/vineet4007/real-time-canary-control-plane/internal/grpc/rolloutpb"
	rolloutv1alpha1 "github.com/vineet4007/real-time-canary-control-plane/internal/k8s/api/v1alpha1"
	"github.com/vineet4007/real-time-canary-control-plane/internal/redis"
)

const (
	defaultRedisAddr = "localhost:6379"
	defaultGRPCAddr  = "127.0.0.1:50051"
)

type grpcStarter struct {
	client rolloutpb.RolloutControlClient
}

func (s *grpcStarter) StartRollout(ctx context.Context, serviceID, version string) error {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.client.StartRollout(callCtx, &rolloutpb.StartRolloutRequest{
		ServiceId: serviceID,
		Version:   version,
	})
	return err
}

type kubernetesDeploymentStatusProvider struct {
	client client.Client
}

func (p *kubernetesDeploymentStatusProvider) GetStatus(
	ctx context.Context,
	namespace,
	serviceID string,
) (*controllerrollout.DeploymentStatus, error) {
	canaryName, stableName := serviceDeploymentNames(serviceID)
	status := &controllerrollout.DeploymentStatus{}

	canary, err := p.readDeployment(ctx, namespace, canaryName)
	if err != nil {
		return nil, err
	}
	if canary != nil {
		status.CanaryDesiredReplicas = desiredReplicas(canary.Spec.Replicas)
		status.CanaryReadyReplicas = int(canary.Status.ReadyReplicas)
	}

	stable, err := p.readDeployment(ctx, namespace, stableName)
	if err != nil {
		return nil, err
	}
	if stable != nil {
		status.StableDesiredReplicas = desiredReplicas(stable.Spec.Replicas)
		status.StableReadyReplicas = int(stable.Status.ReadyReplicas)
	}

	return status, nil
}

func (p *kubernetesDeploymentStatusProvider) readDeployment(
	ctx context.Context,
	namespace,
	name string,
) (*appsv1.Deployment, error) {
	var d appsv1.Deployment
	if err := p.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &d); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &d, nil
}

type CanaryRolloutReconciler struct {
	client.Client
	Domain *controllerrollout.Reconciler
}

func (r *CanaryRolloutReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	var resource rolloutv1alpha1.CanaryRollout
	if err := r.Get(ctx, req.NamespacedName, &resource); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, err
	}

	domainRollout := &controllerrollout.CanaryRollout{
		APIVersion: resource.APIVersion,
		Kind:       resource.Kind,
		Metadata: controllerrollout.ObjectMeta{
			Name:      resource.Name,
			Namespace: resource.Namespace,
		},
		Spec: controllerrollout.CanaryRolloutSpec{
			ServiceID:     resource.Spec.ServiceID,
			TargetVersion: resource.Spec.TargetVersion,
			PolicyRef:     resource.Spec.PolicyRef,
		},
		Status: controllerrollout.CanaryRolloutStatus{
			Phase:           controllerrollout.Phase(resource.Status.Phase),
			Reason:          resource.Status.Reason,
			LastDecision:    resource.Status.LastDecision,
			ObservedVersion: resource.Status.ObservedVersion,
			LastUpdatedUnix: resource.Status.LastUpdatedUnix,
		},
	}

	if err := r.Domain.Reconcile(ctx, domainRollout); err != nil {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, err
	}

	resource.Status.Phase = string(domainRollout.Status.Phase)
	resource.Status.Reason = domainRollout.Status.Reason
	resource.Status.LastDecision = domainRollout.Status.LastDecision
	resource.Status.ObservedVersion = domainRollout.Status.ObservedVersion
	resource.Status.LastUpdatedUnix = domainRollout.Status.LastUpdatedUnix

	if err := r.Status().Update(ctx, &resource); err != nil {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, err
	}

	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *CanaryRolloutReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rolloutv1alpha1.CanaryRollout{}).
		Complete(r)
}

func main() {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	grpcAddr := getEnv("ROLLOUT_CONTROLLER_GRPC_ADDR", defaultGRPCAddr)
	redisAddr := getEnv("ROLLOUT_CONTROLLER_REDIS_ADDR", defaultRedisAddr)

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(rolloutv1alpha1.AddToScheme(scheme))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		ctrl.Log.WithName("setup").Error(err, "failed to create manager")
		os.Exit(1)
	}

	conn, err := grpc.Dial(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		ctrl.Log.WithName("setup").Error(err, "failed to dial decision engine grpc")
		os.Exit(1)
	}
	defer conn.Close()

	domainReconciler := &controllerrollout.Reconciler{
		Starter: &grpcStarter{
			client: rolloutpb.NewRolloutControlClient(conn),
		},
		Store: redis.New(redisAddr),
		Deployments: &kubernetesDeploymentStatusProvider{
			client: mgr.GetClient(),
		},
	}

	if err := (&CanaryRolloutReconciler{
		Client: mgr.GetClient(),
		Domain: domainReconciler,
	}).SetupWithManager(mgr); err != nil {
		ctrl.Log.WithName("setup").Error(err, "unable to create controller")
		os.Exit(1)
	}

	ctrl.Log.WithName("setup").Info(
		"kubernetes rollout controller started",
		"grpcAddr", grpcAddr,
		"redisAddr", redisAddr,
	)

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		ctrl.Log.WithName("setup").Error(err, "manager exited")
		os.Exit(1)
	}
}

func serviceDeploymentNames(serviceID string) (string, string) {
	base := strings.TrimSpace(serviceID)
	if strings.HasSuffix(base, "-service") {
		base = strings.TrimSuffix(base, "-service")
	}
	if base == "" {
		base = "unknown"
	}
	return base + "-canary", base + "-stable"
}

func desiredReplicas(replicas *int32) int {
	if replicas == nil {
		return 1
	}
	return int(*replicas)
}

func getEnv(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}
