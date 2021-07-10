package server

import (
	"container/list"
	"context"
	"os"
	"sync"

	"github.com/Mrs4s/go-cqhttp/coolq"
	"github.com/Mrs4s/go-cqhttp/global"

	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"golang.org/x/time/rate"
)

var (
	filters     = make(map[string]global.Filter)
	filterMutex sync.RWMutex
)

func rateLimit(frequency float64, bucketSize int) handler {
	limiter := rate.NewLimiter(rate.Limit(frequency), bucketSize)
	return func(_ string, _ resultGetter) coolq.MSG {
		_ = limiter.Wait(context.Background())
		return nil
	}
}

func addFilter(file string) {
	if file == "" {
		return
	}
	bs, err := os.ReadFile(file)
	if err != nil {
		log.Error("init filter error: ", err)
		return
	}
	defer func() {
		if err := recover(); err != nil {
			log.Error("init filter error: ", err)
		}
	}()
	filter := global.Generate("and", gjson.ParseBytes(bs))
	filterMutex.Lock()
	filters[file] = filter
	filterMutex.Unlock()
}

func findFilter(file string) global.Filter {
	if file == "" {
		return nil
	}
	filterMutex.RLock()
	defer filterMutex.RUnlock()
	return filters[file]
}

func longPolling(bot *coolq.CQBot, maxSize int) handler {
	var mutex sync.Mutex
	cond := sync.NewCond(&mutex)
	queue := list.New()
	bot.OnEventPush(func(event *coolq.Event) {
		mutex.Lock()
		defer mutex.Unlock()
		queue.PushBack(event.RawMsg)
		for maxSize != 0 && queue.Len() > maxSize {
			queue.Remove(queue.Front())
		}
	})
	return func(action string, _ resultGetter) coolq.MSG {
		if action != "get_updates" {
			return nil
		}
		mutex.Lock()
		defer mutex.Unlock()
		if queue.Len() == 0 {
			cond.Wait()
		}
		ret := make([]interface{}, queue.Len())
		for e := queue.Front(); e != nil; e = e.Next() {
			ret = append(ret, e.Value)
			queue.Remove(e)
		}
		return coolq.OK(ret)
	}
}
