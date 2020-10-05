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
	ao := aggregatorservice.AggregatorOptions{BotOptions: bo}
	aggregatorservice.NewAggregatorService(ao)
	log.Info("Service created")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
}
