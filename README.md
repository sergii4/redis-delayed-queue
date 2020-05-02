# Redis delayed queue

Based on redis doc: [delayed tasks](https://redislabs.com/ebook/part-2-core-concepts/chapter-6-application-components-in-redis/6-4-task-queues/6-4-2-delayed-tasks/), [distributed locks](https://redislabs.com/ebook/part-2-core-concepts/chapter-6-application-components-in-redis/6-2-distributed-locking/)

## Start

For start we use redis cluster instance and go code adds/removes to/from [ZSET](https://redis.io/commands#sorted_set) and SETNX for [distributed locks](https://redislabs.com/ebook/part-2-core-concepts/chapter-6-application-components-in-redis/6-2-distributed-locking/)

### Docker redis cluster instance

[Docker Hub](https://hub.docker.com/r/grokzen/redis-cluster)
[Github](https://github.com/Grokzen/docker-redis-cluster)

### Go code 

Libs

- [Go redis client](https://github.com/go-redis/redis)
- [Go redis distributed lock](https://github.com/bsm/redislock)

## Implementation

Normally when we talk about times in Redis, we usually start talking about ZSETs. What if, for any item we wanted to execute in the future, we added it to a ZSET  with its score being the time when we want it to execute? Then we have to check for items that should be executed now.

Enqueue:
```
func (rq *RedisQueue) Enqueue(uuid string, delay time.Duration) {
	_ = rq.client.ZAdd(rq.key, &redis.Z{Member: uuid, Score: float64(time.Now().Add(delay).Unix())})
}
```
Dequeue:
```
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

```

## Test
To run test:
```
make test-redis-queue 
```
