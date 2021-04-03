package main

import (
	"flag"
	"log"
	"github.com/NicoNex/echotron/v2"
)

type stateFn func(*echotron.Update) stateFn

type bot struct {
	state stateFn
	chatID int64
	echotron.API
}

var (
	tgToken  string
	cmcToken string
)

func newBot(chatID int64) echotron.Bot {
	b := &bot{
		chatID:       chatID,
		API: echotron.NewAPI(tgToken),
	}
	b.state = b.handleMessage
	return b
}

func (b *bot) handleMessage(update *echotron.Update) stateFn {
	return nil
}

func (b *bot) Update(update *echotron.Update) {
	b.state = b.state(update)
}

func main() {
	flag.StringVar(&tgToken, "tg", "", "The Telegram bot token")
	flag.StringVar(&cmcToken, "cmc", "", "The CoinMarketCap api token")
	flag.Parse()

	dsp := echotron.NewDispatcher(tgToken, newBot)
	log.Fatalln(dsp.Poll())
}
