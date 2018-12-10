package teams

import (
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/nais/tobac/pkg/azure"
)

var mutex sync.Mutex
var teamList map[string]azure.Team

// Sync keeps local copy of teamList in sync
func Sync(interval, timeout time.Duration) {
	timer := time.NewTimer(interval)

	for {
		timer.Reset(interval)
		log.Infof("Retrieving teams from MS Graph API")
		ctx, _ := azure.DefaultContext(timeout)
		teams, err := azure.Teams(ctx)
		if err != nil {
			log.Errorf("while retrieving teams: %s", err)
			<-timer.C
			continue
		}
		mutex.Lock()
		teamList = teams
		mutex.Unlock()
		log.Infof("Cached %d teams from Azure AD", len(teamList))
		<-timer.C
	}
}

// Get returns a team with the specified identified
func Get(id string) azure.Team {
	mutex.Lock()
	defer mutex.Unlock()
	return teamList[id]
}
