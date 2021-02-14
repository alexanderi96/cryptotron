package main

import (
	"github.com/NicoNex/echotron"
	"github.com/alexanderi96/cryptotron/config"
	"github.com/alexanderi96/cryptotron/api"
	"fmt"
	"strings"
	"os"
	"log"
	"strconv"
	"flag"
)

type bot struct {
	chatId int64
	echotron.Api
}

func newBot(api echotron.Api, chatId int64) echotron.Bot {
    return &bot{
            chatId,
            api,
        }
}

func (b *bot) Update(update *echotron.Update) {
	path := "./archive/" + strconv.FormatInt(b.chatId, 10) + "/"
	b.checkFolder(path)

	log.Println("Message recieved from: " + strconv.FormatInt(b.chatId, 10))
	if update.Message.Text == "/start" {
	        b.SendMessage(fmt.Sprintf("*Welcome %s!*\nThis bot helps you monitoring cryptocurrencies\n\nTo use it type /data <currency code> <target currency code>\ne.g. /data BTC USD", update.Message.User.FirstName), b.chatId)

	} else if update.Message.Text == "/data" {
		textTmp := strings.Split(update.Message.Text, " ")

		if (len(textTmp) > 2) {
			b.SendMessage(api.CryptonatorGetData(textTmp[1], textTmp[2]), b.chatId)
		} else {
			b.SendMessage("Invalid argument: please specify the cryptocurrency and the target after the command", b.chatId)
		}
	} else if strings.Contains(update.Message.Text, "/market") {
		textTmp := strings.Split(update.Message.Text, " ")

		if (len(textTmp) > 2) {
			b.SendMessage(api.CryptonatorGetMarket(textTmp[1], textTmp[2]), b.chatId)
		} else {
			b.SendMessage("Invalid argument: please specify the cryptocurrency and the target after the command", b.chatId)
		}
	
	} else {
		b.SendMessage("Invalid argument: please chose between the 2 functions /data or /market", b.chatId)
	}
}

func (b *bot) checkFolder(path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		
		os.Mkdir(path, 0755)
		log.Println("Directory for user " + strconv.FormatInt(b.chatId, 10) + " created")
	}
}

func main() {

	values, err := config.ReadConfig("config.json")
	var token string

	if err != nil {
		flag.StringVar(&token, "token", "", "The telegram bot token")
		flag.Parse()
		log.Println("Token is: " + token)
		values.token = token
	}

	if _, err := os.Stat("archive"); os.IsNotExist(err) {
		os.Mkdir("archive", 0755)
	}

	dsp := echotron.NewDispatcher(values.token, newBot)
	log.Println("Running cryptotron")
	dsp.Run()
}
