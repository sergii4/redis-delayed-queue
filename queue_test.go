package redisqueue

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bsm/redislock"
	"github.com/go-redis/redis/v7"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/require"
)

func TestQueueLock(t *testing.T) {
	t.Run("same number enqueued/dequeued, 0 delayed task left in queue", func(t *testing.T) {
		batch := 20
		numItems := 100
		ttl := 100 * time.Millisecond
		q := setUpQueue(t, uuid.NewV4().String(), batch, ttl)
		// arange
		for i := 0; i < numItems; i++ {
			q.Enqueue("item"+uuid.NewV4().String(), 0)
		}
		ch := make(chan Message, numItems)
		var wg sync.WaitGroup
		numOfWorkers := 8
		wg.Add(numOfWorkers)
		fail := make(chan error)
		// act
		// run concurrently the same number goroutines as items
		go func() {
			for i := 0; i < numOfWorkers; i++ {
				// allows goroutins deque enough messages
				time.Sleep(5 * ttl)
				go func() {
					defer wg.Done()
					ms, err := q.Dequeue()
					if err != nil {
						fail <- err
						return
					}
					for _, m := range ms {
						ch <- m
					}
				}()
			}
		}()
		go func() {
			// close items when every goroutine is executed
			wg.Wait()
			close(ch)
		}()

		count := 0
		done := make(chan struct{})
		// read from message channel
		go func() {
			for m := range ch {
				m.OnProcessed()
				count++
			}
			close(done)
		}()
		// wait for done, fail or timeout
		select {
		case <-done:
		case err := <-fail:
			t.Fatal(err)
		case <-time.After(10 * time.Second):
			t.Fatal("timeout")
		}
		// assert
		require.Equal(t, numItems, count)
	})
	t.Run("first routine processed all messeges last none", func(t *testing.T) {
		numItems := 5
		ttl := 100 * time.Millisecond
		q := setUpQueue(t, uuid.NewV4().String(), numItems, ttl)
		// arange
		for i := 0; i < numItems; i++ {
			q.Enqueue("item"+uuid.NewV4().String(), 0)
		}
		for i := 0; i < numItems; i++ {
			q.Enqueue("item"+uuid.NewV4().String(), 5*time.Second)
		}
		ms, err := q.Dequeue()
		if err != nil {
			require.NoError(t, err)
		}
		require.Len(t, ms, numItems)
	})

}

func TestQueueuBecameEmpty(t *testing.T) {
	numItems := 10
	ttl := 100 * time.Millisecond
	q := setUpQueue(t, uuid.NewV4().String(), numItems, ttl)
	// arange
	for i := 0; i < numItems; i++ {
		q.Enqueue("item"+uuid.NewV4().String(), 0)
	}

	ms, err := q.Dequeue()
	require.NoError(t, err)
	require.Equal(t, numItems, len(ms))
	for _, msg := range ms {
		msg.OnProcessed()
	}

	ms, err = q.Dequeue()
	require.NoError(t, err)
	require.Equal(t, 0, len(ms))
}

func setUpQueue(t *testing.T, key string, batch int, ttl time.Duration) Queue {
	client := setRedisClusterClient(t)
	locker := redislock.New(client)

	return NewQueue(client, locker, key, batch, ttl)
}

func setRedisClusterClient(t *testing.T) *redis.ClusterClient {
	client := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs: []string{"testredis:7000", "testredis:7001"},
	})

	waitOKState(t, client)

	return client
}

func waitOKState(t *testing.T, client *redis.ClusterClient) {
	done := make(chan struct{})
	fail := make(chan error)
	go func() {
		for {
			info, err := client.ClusterInfo().Result()
			if err != nil {
				fail <- err
				return
			}
			if strings.HasPrefix(info, "cluster_state:ok") {
				// fmt.Println(info)
				close(done)
				return
			}
		}
	}()
	select {
	case err := <-fail:
		t.Fatalf("cannot connect to redis instance: %s", err)
	case <-time.After(20 * time.Second):
		t.Fatal("cluster state: fail")
	case <-done:
	}
}
