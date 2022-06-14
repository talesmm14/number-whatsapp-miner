package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal"
	"github.com/talesmm14/number-whatsapp-miner/config"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	"os"
	"sync/atomic"
	"time"
)

var (
	cli           *whatsmeow.Client
	log           waLog.Logger
	historySyncID int32
	startupTime   = time.Now().Unix()
)

const logLevel = "DEBUG"

func main() {
	log = waLog.Stdout("Main", logLevel, true)
	dbLog := waLog.Stdout("Database", logLevel, true)
	storeContainer, err := sqlstore.New("sqlite3", fmt.Sprintf("file:%s/%s?_foreign_keys=off", config.PathDB, config.DBName), dbLog)
	if err != nil {
		log.Errorf("Failed to connect to database: %v", err)
		panic(err)
	}

	device, err := storeContainer.GetFirstDevice()
	if err != nil {
		log.Errorf("Failed to get device: %v", err)
		panic(err)
	}

	store.CompanionProps.PlatformType = waProto.CompanionProps_UNKNOWN.Enum()
	store.CompanionProps.Os = proto.String(config.AppName)

	cli = whatsmeow.NewClient(device, waLog.Stdout("Client", logLevel, true))
	cli.AddEventHandler(eventHandler)

	ch, err := cli.GetQRChannel(context.Background())
	if err != nil {
		if !errors.Is(err, whatsmeow.ErrQRStoreContainsID) {
			log.Errorf("Failed to get QR channel: %v", err)
		}
	} else {
		go func() {
			for evt := range ch {
				if evt.Event == "code" {
					qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				} else {
					log.Infof("QR channel result: %s", evt.Event)
				}
			}
		}()
	}

	err = cli.Connect()
	if err != nil {
		log.Errorf("Failed to connect: %v", err)
		return
	}
}

func eventHandler(rawEvt interface{}) {
	switch evt := rawEvt.(type) {
	case *events.Message:
		fmt.Println(evt.Message.GetConversation())
	case *events.AppStateSyncComplete:
		if len(cli.Store.PushName) > 0 && evt.Name == appstate.WAPatchCriticalBlock {
			err := cli.SendPresence(types.PresenceAvailable)
			if err != nil {
				log.Warnf("Failed to send available presence: %v", err)
			} else {
				log.Infof("Marked self as available")
			}
		}
	case *events.Connected, *events.PushNameSetting:
		if len(cli.Store.PushName) == 0 {
			return
		}
		// Send presence available when connecting and when the pushname is changed.
		// This makes sure that outgoing messages always have the right pushname.
		err := cli.SendPresence(types.PresenceAvailable)
		if err != nil {
			log.Warnf("Failed to send available presence: %v", err)
		} else {
			log.Infof("Marked self as available")
		}
	case *events.StreamReplaced:
		os.Exit(0)
	case *events.HistorySync:
		id := atomic.AddInt32(&historySyncID, 1)
		fileName := fmt.Sprintf("%s/history-%d-%d.json", config.PathDB, startupTime, id)
		file, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE, 0600)
		if err != nil {
			log.Errorf("Failed to open file to write history sync: %v", err)
			return
		}
		enc := json.NewEncoder(file)
		enc.SetIndent("", "  ")
		err = enc.Encode(evt.Data)
		if err != nil {
			log.Errorf("Failed to write history sync: %v", err)
			return
		}
		log.Infof("Wrote history sync to %s", fileName)
		_ = file.Close()
	case *events.AppState:
		log.Debugf("App state event: %+v / %+v", evt.Index, evt.SyncActionValue)
	}
}
