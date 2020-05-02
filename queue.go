package redisqueue

import (
	"fmt"
	"time"

	"github.com/bsm/redislock"
	"github.com/go-redis/redis/v7"
	"github.com/pkg/errors"
)

type Message struct {
	Message     string
	OnProcessed func()
}

type Queue interface {
	Enqueue(message string, after time.Duration)
	Dequeue() ([]Message, error)
}

// RedisQueue implements Redis Delayed tasks algorith: https://redislabs.com/ebook/part-2-core-concepts/chapter-6-application-components-in-redis/6-4-task-queues/6-4-2-delayed-tasks/
type RedisQueue struct {
	client *redis.ClusterClient
	locker *redislock.Client
	key    string
	batch  int
	ttl    time.Duration
}

func NewQueue(client *redis.ClusterClient, locker *redislock.Client, key string, batch int, ttl time.Duration) Queue {
	return &RedisQueue{client: client, locker: locker, key: key, batch: batch, ttl: ttl}
}

// Enqueue puts task/item with uuid to the queue/container with delay
func (rq *RedisQueue) Enqueue(uuid string, delay time.Duration) {
	_ = rq.client.ZAdd(rq.key, &redis.Z{Member: uuid, Score: float64(time.Now().Add(delay).Unix())})
}

// Dequeue receives batch of messages from the queue
func (rq *RedisQueue) Dequeue() ([]Message, error) {
	var ms []Message
	start := int64(0)
	for i := rq.batch; i >= 0; {
		vals, err := rq.client.ZRangeWithScores(rq.key, start, start).Result()
		if err != nil {
			return nil, errors.Wrap(err, "cannot get range from zset")
		}
		if len(vals) == 0 || vals[0].Score > float64(time.Now().Unix()) {
			break
		}

		id := vals[0].Member.(string)
		lock := rq.acquireLock(id)
		if lock == nil {
			start++
			continue
		}
		ms = append(ms, Message{Message: id, OnProcessed: func() {
			_ = rq.client.ZRem(rq.key, id)
			if err := lock.Release(); err != nil {
				fmt.Printf("release lock erros = %+v\n", err)
			}
		}})
		start++
		i--

	}

	return ms, nil

}

// acquireLock gets the lock for the item with key
func (rq *RedisQueue) acquireLock(key string) *redislock.Lock {
	lock, err := rq.locker.Obtain(key, 1000*time.Millisecond, nil)
	if err != nil {
		return nil
	}
	return lock
}
