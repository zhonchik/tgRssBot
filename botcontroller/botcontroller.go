package botcontroller

import (
	log "github.com/sirupsen/logrus"
	tb "gopkg.in/tucnak/telebot.v3"
	"time"
)

type (
	BotOptions struct {
		Token string
	}

	BotController struct {
		Bot tb.Bot
	}
)

func NewBotController(options BotOptions) *BotController {
	b, err := tb.NewBot(tb.Settings{
		Token:  options.Token,
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		log.Fatal(err)
		return nil
	}
	bot := &BotController{
		Bot: *b,
	}

	b.Handle("/start", func(c tb.Context) error {
		log.Infof("Start command received from %+v", c.Sender())
		return c.Reply("Hello!")
	})

	go func() {
		b.Start()
	}()
	return bot
}

func (bc *BotController) AddHandler(command string, handler func(ctx tb.Context) error) {
	log.Infof("Adding handler for %s command", command)
	bc.Bot.Handle(command, handler)
}

func (bc *BotController) SendTextMessage(chatID int64, message string) error {
	chat := tb.Chat{
		ID: chatID,
	}
	_, err := bc.Bot.Send(&chat, message)
	if err != nil {
		log.Warnf("Failed to send message: %s", err)
	}
	return err
}
