package teams

import (
	"github.com/sirupsen/logrus"
	"sync"
	"time"

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
		timer.Reset(interval)
		logrus.Infof("Retrieving teams from MS Graph API")
		teams, err := azure.Teams(ctx)
		if err != nil {
			logrus.Errorf("while retrieving teams: %s", err)
			<-timer.C
			continue
		}
		mutex.Lock()
		teamList = teams
		mutex.Unlock()
		logrus.Infof("Cached %d teams from Azure AD", len(teamList))
		<-timer.C
	}
}

// Get returns a team with the specified identified
func Get(id string) azure.Team {
	mutex.Lock()
	defer mutex.Unlock()
	return teamList[id]
}
