package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"

	"github.com/gomodule/redigo/redis"
	_ "github.com/lib/pq"
)

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

func newPostgreClient() *sql.DB {
	// All of the parameters needed to connect to Postgre.
	pgUser := os.Getenv("PGUSER")
	pgPassword := os.Getenv("PGPASSWORD")
	pgHost := os.Getenv("PGHOST")
	pgPort := os.Getenv("PGPORT")
	pgDatabase := os.Getenv("PGDATABASE")
	pgSSLMode := os.Getenv("PGSSLMODE")

	connStr := fmt.Sprintf("user=%s password=%s dbname=%s host=%s port=%s sslmode=%s",
		pgUser, pgPassword, pgDatabase, pgHost, pgPort, pgSSLMode)
	fmt.Println("Postgres connection string:", connStr)

	// Create a postgres db connection.
	conn, err := sql.Open("postgres", connStr)
	if err != nil {
		panic(err)
	}

	return conn
}

// Make the endpoint accessible in JavaScript browser code, like React.
func enableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
}

// Return Hello world.
func helloWorldHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	fmt.Fprintln(w, "Hello world.")
}

// Return all the records in the "values" table in Postgres.
// Send back response with a JSON encoded array.
func fetchPostgresDataHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)

	// Use Goroutine to make fetching Postgres data parallel.
	ch := make(chan []int, 1)
	go func() {
		rows, _ := db.Query("SELECT * FROM values;")
		defer rows.Close()

		// Parse the rows into []int. Or we can also let the frontend parse it.
		var rowsInt = make([]int, 0)
		for rows.Next() {
			var number int
			err := rows.Scan(&number)
			if err != nil {
				panic(err)
			}
			rowsInt = append(rowsInt, number)
		}
		ch <- rowsInt
	}()

	rows := <-ch
	rowsJSON, err := json.Marshal(rows)
	if err != nil {
		panic(err)
	}
	fmt.Fprintf(w, "%s", rowsJSON)
}

// Return all of the records in Redis "values" keyset.
// Send back response with a JSON encoded object.
func fetchRedisDataHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)

	// Use Goroutine to make fetching Redis data parallel.
	ch := make(chan map[string]string, 1)
	go func() {
		result, err := r2.Do("HGETALL", "values")
		if err != nil {
			panic(err)
		}

		// Parse the HGETALL result to a map.
		vs, err := redis.StringMap(result, err)
		if err != nil {
			panic(err)
		}

		ch <- vs
	}()

	values := <-ch
	valuesJSON, err := json.Marshal(values)
	if err != nil {
		panic(err)
	}

	fmt.Fprintf(w, "%s", valuesJSON)
}

type message struct {
	Index string `json:"index"`
}

// Handles submitting a Fibonacci index, and let the worker calculate the value.
// curl -d '{"index":"5"}' -X POST -i -H "Content-Type:application/json" http://localhost:3050/values
// By default, React Axios module sends body data in application/json content type.
func submitFibIndexHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)

	// This endpoint only handles POST request.
	switch r.Method {
	case "POST":
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}

		// Parse the index out of request body.
		var msg message
		err = json.Unmarshal(body, &msg)
		if err != nil {
			panic(err)
		}
		fmt.Println("Index is:", msg.Index)

		// Limit Fibonacci calculation to the first 40 index.
		index, err := strconv.Atoi(msg.Index)
		if err != nil {
			panic(err)
		}
		if index > 40 {
			http.Error(w, "Index too high", http.StatusBadRequest)
			return
		}

		// Set the index in both Redis and Postgres.
		r2.Do("HSET", "values", index, "Nothing yet!")
		r1.Do("PUBLISH", "insert", index)
		db.Exec("INSERT INTO values(number) VALUES($1)", index)

		// Send response back to client.
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{working: true}"))
	default:
		fmt.Fprintln(w, "Only POST request is supported at this endpoint.")
	}
}

// Start Redis client connection.
// R1 Redis client is for Redis pub/sub
var r1 = *newRedisClient()

// R2 Redis client is for Redis get/set
var r2 = *newRedisClient()

// Start Postgre client connection.
var db = newPostgreClient()

// Create the initial table in Postgres.
func setupPostgres() {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS values (number int);")
	if err != nil {
		panic(err)
	}
}

func main() {
	defer r1.Close()
	defer r2.Close()
	defer db.Close()
	setupPostgres()

	// Set up route handling.
	http.HandleFunc("/", helloWorldHandler)
	http.HandleFunc("/values/all", fetchPostgresDataHandler)
	http.HandleFunc("/values/current", fetchRedisDataHandler)
	http.HandleFunc("/values", submitFibIndexHandler)

	// Start http server.
	fmt.Println("Started server service.")
	http.ListenAndServe(":"+os.Getenv("HTTP_PORT"), nil)
}
