package main

import (
	"fmt"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/utils"
	"go.mau.fi/whatsmeow/types/events"
)

func eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		fmt.Println(v.Message.GetConversation())
	}
}

func main() {
	db := utils.InitWaDB()
	client := utils.InitWaCLI(db)

	client.AddEventHandler(eventHandler)
}
