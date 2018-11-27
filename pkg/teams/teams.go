package teams

import (
	"github.com/golang/glog"
	"github.com/nais/tobac/pkg/azure"
	"sync"
	"time"
)

var mutex sync.Mutex
var teamList map[string]azure.Team

func Sync(interval time.Duration) {
	ctx, cancelFunc := azure.DefaultContext()
	timer := time.NewTimer(interval)
	defer cancelFunc()

	for {
		glog.V(9).Infof("Retrieving teams from MS Graph API")
		teams, err := azure.Teams(ctx)
		if err != nil {
			glog.Errorf("while retrieving teams: %s", err)
			<-timer.C
			continue
		}
		mutex.Lock()
		teamList = teams
		mutex.Unlock()
		glog.Infof("Cached %d teams from Azure AD", len(teamList))
		<-timer.C
	}
}

func Get(id string) azure.Team {
	mutex.Lock()
	defer mutex.Unlock()
	return teamList[id]
}
