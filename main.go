package main

import (
	"context"
	"github.com/heetch/confita"
	"github.com/heetch/confita/backend/file"
	"github.com/lestrrat-go/file-rotatelogs"
	log "github.com/sirupsen/logrus"
	"io"
	"os"
	"os/signal"
	"tgRssBot/aggregatorservice"
	"tgRssBot/botcontroller"
	"tgRssBot/storage"
)

type (
	BotConfig struct {
		Token string `config:"token,required"`
	}

	Config struct {
		Bot BotConfig `config:"bot"`
	}
)

func main() {
	err := configureLogger()
	if err != nil {
		log.Errorf("Failed to configure logger: %s", err)
		return
	}

	cfg := Config{Bot: BotConfig{Token: ""}}
	loader := confita.NewLoader(
		file.NewBackend("./config.yaml"),
	)
	err = loader.Load(context.Background(), &cfg)
	if err != nil {
		log.Errorf("Failed to load config: %s", err)
		return
	}

	bo := botcontroller.BotOptions{Token: cfg.Bot.Token}
	bc := botcontroller.NewBotController(bo)
	log.Info("Bot created")

	so := storage.Options{Path: "./storage.db"}
	s, err := storage.NewStorage(so)
	if err != nil {
		log.Errorf("Failed to create storage: %s", err)
		return
	}

	as, err := aggregatorservice.NewAggregatorService(bc.SendTextMessage, s)
	if err != nil {
		log.Errorf("Failed to create service: %s", err)
		return
	}
	log.Info("Service created")
	defer func() {
		err := as.Close()
		if err == nil {
			log.Info("Service closed successfully")
		} else {
			log.Errorf("Error on service closing: %s", err)
		}
	}()

	bc.AddHandler(botcontroller.CommandStart{AggregatorService: as, BotController: bc})
	bc.AddHandler(botcontroller.CommandSubscribe{AggregatorService: as, BotController: bc})
	bc.AddHandler(botcontroller.CommandUnsubscribe{AggregatorService: as, BotController: bc})
	bc.AddHandler(botcontroller.CommandList{AggregatorService: as, BotController: bc})

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
}

func configureLogger() error {
	rl, err := rotatelogs.New("./logs/app_log.%Y%m%d%H%M")
	if err != nil {
		return err
	}
	mw := io.MultiWriter(os.Stdout, rl)
	log.SetOutput(mw)
	customFormatter := new(log.TextFormatter)
	customFormatter.TimestampFormat = "2006-01-02 15:04:05"
	customFormatter.FullTimestamp = true
	log.SetFormatter(customFormatter)
	return nil
}
