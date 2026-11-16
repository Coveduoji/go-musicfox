package likelist

import (
	"strconv"
	"sync"

	"github.com/buger/jsonparser"
	"github.com/go-musicfox/netease-music/service"
)

type LikeList map[int64]struct{}

var (
	likeList = make(LikeList)
	mu       sync.Mutex
)

func IsLikeSong(songId int64) bool {
	mu.Lock()
	defer mu.Unlock()
	_, ok := likeList[songId]
	return ok
}

// SetLikeSong updates the local state only after the server confirms the change.
func SetLikeSong(songID int64, liked bool) {
	mu.Lock()
	defer mu.Unlock()
	if liked {
		likeList[songID] = struct{}{}
	} else {
		delete(likeList, songID)
	}
}

func RefreshLikeList(userId int64) {
	s := &service.LikeListService{UID: strconv.FormatInt(userId, 10)}
	code, resp := s.LikeList()
	if code != 200 {
		return
	}

	mu.Lock()
	defer mu.Unlock()
	likeList = make(LikeList)
	_, _ = jsonparser.ArrayEach(resp, func(value []byte, _ jsonparser.ValueType, _ int, err error) {
		if err != nil {
			return
		}
		if id, err := jsonparser.ParseInt(value); err == nil {
			likeList[id] = struct{}{}
		}
	}, "ids")
}
