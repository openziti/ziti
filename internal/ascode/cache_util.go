package common

import (
	"errors"
	"github.com/patrickmn/go-cache"
	"log/slog"
)

type CacheGetter func(id string) (interface{}, error)

func GetItemFromCache(c *cache.Cache, key string, fn CacheGetter) (interface{}, error) {
	if key == "" {
		return nil, errors.New("key is null, can't resolve from cache or get it from source")
	}
	detail, found := c.Get(key)
	if !found {
		slog.
			With("key", key).
			Debug("Item not in cache, getting from source")
		var err error
		detail, err = fn(key)
		if err != nil {
			return nil, errors.Join(errors.New("error reading: "+key), err)
		}
		c.Set(key, detail, cache.NoExpiration)
		return detail, nil
	}
	slog.
		With("key", key).
		Debug("Item found in cache")
	return detail, nil
}
