package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/gomodule/redigo/redis"
)

// Fibonacci calculation with recursion.
func fib(index int) int {
	if index < 2 {
		return 1
	}
	return fib(index-1) + fib(index-2)
}

// Create a new Redis client connection.
func newRedisClient() *redis.Conn {
	redisCon := os.Getenv("REDIS_CON_TYPE")
	redisHost := os.Getenv("REDIS_HOST")
	redisPort := os.Getenv("REDIS_PORT")

	pool := redis.Pool{
		MaxIdle:   10,
		MaxActive: 20, // max number of connections
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial(redisCon, redisHost+":"+redisPort)
			if err != nil {
				panic(err)
			}
			return c, err
		},
	}
	redisClient := pool.Get()

	return &redisClient
}

func main() {
	fmt.Println("Started worker service.")

	// Start Redis client connection.
	// C1 Redis client is for Redis subscribe operation
	c1 := *newRedisClient()
	defer c1.Close()
	// C2 Redis client is for Redis set operation
	c2 := *newRedisClient()
	defer c2.Close()

	// Subscribe to "insert" channel.
	c1.Do("SUBSCRIBE", "insert")
	// Listening to the channel and receive the message published on the channel.
	// c1.Receive() will blocking wait for the message, once a message is received it will exit.
	for {
		rawMsg, err := c1.Receive()
		if err != nil {
			panic(err)
		}

		// Once a message is received, start a new goroutine to handle the message.
		go func() {
			// Parse the message, and get the integer index.
			msgArr := rawMsg.([]interface{})
			channel := string(msgArr[1].([]byte))
			message := string(msgArr[2].([]byte))
			fmt.Println("Message received:", message, "from channel:", channel)
			index, err := strconv.Atoi(message)
			if err != nil {
				panic(err)
			}

			// Calculate the fib number corresponding to the index.
			result := fib(index)
			fmt.Println("The result of fib calculation is:", result)

			// Then write the index-result key-value pair to Redis.
			c2.Do("HSET", "values", index, result)
		}()
	}
}
