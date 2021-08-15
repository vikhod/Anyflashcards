package main

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"

var statuses = map[string]string{
	"administrator": "valid",
	"creator":       "valid",
	"kicked":        "invalid",
	"left":          "invalid",
	"member":        "valid",
	"restricted":    "invalid",
}

func updateFrom(update *tgbotapi.Update) (updateInitiatorUser *tgbotapi.User) {
	if update.CallbackQuery != nil {
		return update.CallbackQuery.From

	} else if update.ChannelPost != nil {
		return update.ChannelPost.From

	} else if update.ChosenInlineResult != nil {
		return update.ChosenInlineResult.From

	} else if update.EditedChannelPost != nil {
		return update.EditedChannelPost.From

	} else if update.EditedMessage != nil {
		return update.EditedMessage.From

	} else if update.InlineQuery != nil {
		return update.InlineQuery.From

	} else if update.Message != nil {
		return update.Message.From

	} else if update.PreCheckoutQuery != nil {
		return update.PreCheckoutQuery.From

	} else if update.ShippingQuery != nil {
		return update.ShippingQuery.From
	} else {
		return nil
	}

}

/*
func contains(array []int, number int) bool {
	for _, value := range array {
		if value == number {
			return true
		}
	}

	return false
}
*/
