package main

import (
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/juju/mgosession"
	mgo "gopkg.in/mgo.v2"
)

func main() {

	env := os.Getenv("BOOKMARK_ENV")

	if env == "" {
		env = "dev"
	}

	err := godotenv.Load("config/" + env + ".env")

	if err != nil {
		log.Fatal("Error loading .env file")
	}

	session, err := mgo.Dial(os.Getenv("MONGODB_HOST"))
	if err != nil {
		log.Fatal(err.Error())
	}

	defer session.Close()

	r := mux.NewRouter()

	cPool, err := strconv.Atoi(os.Getenv("MONGODB_CONNECTION_POOL"))

	if err != nil {
		log.Println(err.Error())
		cPool = 10
	}

	mPool := mgosession.NewPool(nil, session, cPool)
	defer mPool.Close()

	http.Handle("/", r)

}
