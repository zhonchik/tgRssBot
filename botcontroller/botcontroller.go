package botcontroller

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	tb "gopkg.in/tucnak/telebot.v3"
	"strconv"
	"strings"
	"time"
)

type (
	BotOptions struct {
		Token string
	}

	BotController struct {
		bot tb.Bot
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
	bc := &BotController{
		bot: *b,
	}

	b.Handle("/start", func(c tb.Context) error {
		log.Infof("Start command received from %+v", c.Sender())
		return c.Reply("Hello!")
	})

	go func() {
		b.Start()
	}()
	return bc
}

func (bc *BotController) AddHandler(command Command) {
	log.Infof("Adding handler for %s command", command.GetCommand())
	bc.bot.Handle(command.GetCommand(), command.Handler)
}

func (bc *BotController) SendTextMessage(chatID int64, message string) error {
	chat := tb.Chat{
		ID: chatID,
	}
	_, err := bc.bot.Send(&chat, message)
	if err != nil {
		log.Warnf("Failed to send message: %s", err)
	}
	return err
}

func (bc *BotController) HandleMultiArgCommand(ctx tb.Context, handler func(chatID int64, arg string) error) error {
	sender := ctx.Sender()
	log.Infof("%s command received from %+v", ctx.Message().Text, sender)
	chatID, err := bc.getChatID(sender)
	if err != nil {
		return err
	}
	var lines []string
	for _, arg := range ctx.Args() {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		err := handler(chatID, arg)
		var line string
		if err == nil {
			line = fmt.Sprintf("✅ %s", arg)
		} else {
			line = fmt.Sprintf("⚠️ %s %s", arg, err)
		}
		lines = append(lines, line)
	}
	return bc.SendTextMessage(chatID, strings.Join(lines, "\n"))
}

func (bc *BotController) getChatID(sender tb.Recipient) (int64, error) {
	chatID, err := strconv.ParseInt(sender.Recipient(), 10, 64)
	if err != nil {
		log.Warnf("Failed to get chat id: %s", err)
		return 0, err
	}
	return chatID, nil
}
