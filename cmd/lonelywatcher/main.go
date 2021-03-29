package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/go-resty/resty/v2"
	"github.com/hack-fan/config"
	"github.com/hack-fan/x/xdb"
	"github.com/hack-fan/x/xlog"

	"github.com/hack-fan/skadi/service"
	"github.com/hack-fan/skadi/types"
)

func main() {
	// load config
	var settings = new(Settings)
	config.MustLoad(settings)

	// logger
	var logger = xlog.New(settings.Debug, settings.Wework)
	defer logger.Sync() // nolint
	var log = logger.Sugar()

	// kv
	var kv = redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", settings.Redis.Host, settings.Redis.Port),
		Password: settings.Redis.Password,
		DB:       settings.Redis.DB,
	})

	// db
	xdb.SetLogger(log)
	var db = xdb.New(settings.DB)
	if settings.Debug {
		db = db.Debug()
	}
	// auto create table
	go db.AutoMigrate(&types.Job{}) // nolint

	// http client
	var rest = resty.New().SetRetryCount(3).
		SetRetryWaitTime(5 * time.Second).
		SetRetryMaxWaitTime(60 * time.Second)

	// service
	var s = service.New(kv, db, rest, log)

	// watch redis expire events
	ctx := context.Background()
	pubsub := kv.Subscribe(ctx, "__keyevent@0__:expired")
	log.Info("start watching redis key expired event...")
	log.Warnf("Skadi Watcher started: %s", settings.Hostname)
	for msg := range pubsub.Channel() {
		key := msg.Payload
		log.Debugw("redis key expired", "key", key)
		if strings.HasPrefix(key, "job:wait:") {
			jid := strings.TrimPrefix(key, "job:wait:")
			go s.JobExpire(jid)
		} else if strings.HasPrefix(key, "agent:online:") {
			aid := strings.TrimPrefix(key, "agent:online:")
			go s.AgentOffline(aid)
		}
	}
	log.Errorf("Skadi Watcher start failed: %s", settings.Hostname)
}
