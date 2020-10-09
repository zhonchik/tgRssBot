package main

import (
	"context"
	"github.com/heetch/confita"
	"github.com/heetch/confita/backend/file"
	log "github.com/sirupsen/logrus"
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
	cfg := Config{Bot: BotConfig{Token: ""}}
	loader := confita.NewLoader(
		file.NewBackend("./config.yaml"),
	)
	err := loader.Load(context.Background(), &cfg)
	if err != nil {
		log.Errorf("Failed to load config", err)
		return
	}

	bo := botcontroller.BotOptions{Token: cfg.Bot.Token}
	so := storage.Options{Path: "./storage.db"}
	ao := aggregatorservice.AggregatorOptions{BotOptions: bo, StorageOptions: so}

	as, err := aggregatorservice.NewAggregatorService(ao)
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

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
}
