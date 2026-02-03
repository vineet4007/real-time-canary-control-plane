package redis

import (
	"context"
	"encoding/json"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type RolloutState string

const (
	Canary     RolloutState = "CANARY"
	Promoted   RolloutState = "PROMOTED"
	Paused     RolloutState = "PAUSED"
	RolledBack RolloutState = "ROLLED_BACK"
)

type State struct {
	ServiceID    string       `json:"service_id"`
	Version      string       `json:"version"`
	State        RolloutState `json:"state"`
	LastDecision string       `json:"last_decision"`
	LastUpdated  int64        `json:"last_updated"`
}

type Store struct {
	client *goredis.Client
}

func New(addr string) *Store {
	return &Store{
		client: goredis.NewClient(&goredis.Options{
			Addr: addr,
		}),
	}
}

func (s *Store) Get(ctx context.Context, serviceID string) (*State, error) {
	val, err := s.client.Get(ctx, rolloutKey(serviceID)).Result()
	if err == goredis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var st State
	if err := json.Unmarshal([]byte(val), &st); err != nil {
		return nil, err
	}

	return &st, nil
}

func (s *Store) Save(ctx context.Context, st *State) error {
	st.LastUpdated = time.Now().UnixMilli()
	bytes, _ := json.Marshal(st)
	return s.client.Set(ctx, rolloutKey(st.ServiceID), bytes, 0).Err()
}

func (s *Store) IdempotentDecision(
	ctx context.Context,
	serviceID string,
	windowID string,
) (bool, error) {
	key := "decision:" + serviceID + ":" + windowID
	ok, err := s.client.SetNX(ctx, key, "1", 2*time.Minute).Result()
	return ok, err
}

func rolloutKey(serviceID string) string {
	return "rollout:" + serviceID
}
