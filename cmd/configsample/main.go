package main

import (
	"admin"
	"configmanager"
	"context"
	"fmt"
	"log"
	"net/http"
	"obs"
	"strconv"
)

var client configmanager.Client

func init() {
	c, err := configmanager.NewClient("/etc/configs", "configsample", obs.NullFR)
	if err != nil {
		panic(err)
	}
	client = c
}

func handler(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	rawValue, err := client.GetRaw(key)
	fmt.Fprintf(w, "config value %s error %v!", string(rawValue), err)
}

func whitelist(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	pid := r.URL.Query().Get("project_id")
	projectID, err := strconv.ParseInt(pid, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf("Could not parse project ID %v, err %v", pid, err)))
	}
	fmt.Fprintf(w, "Config value %v", client.IsProjectWhitelisted(key, projectID, false))
}

type blahApp struct {
}

func (a *blahApp) Health(ctx context.Context) error {
	return nil
}
func main() {
	app := &blahApp{}
	adminHandler := admin.Init(app)
	defer adminHandler.Close()

	http.HandleFunc("/config", handler)
	http.HandleFunc("/whitelist", whitelist)
	log.Fatal(http.ListenAndServe(":8000", nil))
}
