package botcontroller

import tb "gopkg.in/tucnak/telebot.v3"

type Command interface {
	GetCommand() string
	GetDescription() string
	Handler(ctx tb.Context) error
}
