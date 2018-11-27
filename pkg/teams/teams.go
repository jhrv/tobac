package teams

import (
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/nais/tobac/pkg/azure"
)

var mutex sync.Mutex
var teamList map[string]azure.Team

// Sync keeps local copy of teamList in sync
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

// Get returns a team with the specified identified
func Get(id string) azure.Team {
	mutex.Lock()
	defer mutex.Unlock()
	return teamList[id]
}
