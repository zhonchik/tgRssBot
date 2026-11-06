package main

import (
	"context"
	"github.com/heetch/confita"
	"github.com/heetch/confita/backend/file"
	"github.com/lestrrat-go/file-rotatelogs"
	log "github.com/sirupsen/logrus"
	"io"
	"net/http"
	"os"
	"os/signal"
	"tgRssBot/aggregatorservice"
	"tgRssBot/botcontroller"
	"tgRssBot/settings"
	"tgRssBot/storage"
	"time"
)

type (
	BotConfig struct {
		SettingsURL    string `config:"settings_url"`
		SettingsListen string `config:"settings_listen"`
		Token          string `config:"token,required"`
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

	as, err := aggregatorservice.NewAggregatorService(bc.SendArticle, s)
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
	for _, paused := range []bool{true, false} {
		bc.AddHandler(botcontroller.CommandPause{AggregatorService: as, BotController: bc, Paused: paused})
	}
	bc.AddHandler(botcontroller.CommandList{AggregatorService: as, BotController: bc})

	if cfg.Bot.SettingsURL != "" {
		addr := cfg.Bot.SettingsListen
		if addr == "" {
			addr = "127.0.0.1:8080"
		}
		server := &http.Server{Addr: addr, Handler: settings.Handler(as, cfg.Bot.Token), ReadHeaderTimeout: 5 * time.Second}
		go func() {
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Errorf("Settings server: %s", err)
			}
		}()
		defer server.Close()

	}
	settingsCommand := &botcontroller.CommandSettings{BotController: bc, AggregatorService: as, URL: cfg.Bot.SettingsURL}
	settingsCommand.Register()
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
