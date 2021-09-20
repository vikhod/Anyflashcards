package main

import (
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

var statuses = map[string]string{
	"administrator": "valid",
	"creator":       "valid",
	"kicked":        "invalid",
	"left":          "invalid",
	"member":        "valid",
	"restricted":    "invalid",
}

/*
func getInitiatorUser(update *tgbotapi.Update) (updateInitiatorUser *tgbotapi.User) {
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
*/
func checkMembership(bot *tgbotapi.BotAPI, update *tgbotapi.Update) bool {

	// Get initiator from any update
	var user *tgbotapi.User
	if update.CallbackQuery != nil {
		user = update.CallbackQuery.From

	} else if update.ChannelPost != nil {
		user = update.ChannelPost.From

	} else if update.ChosenInlineResult != nil {
		user = update.ChosenInlineResult.From

	} else if update.EditedChannelPost != nil {
		user = update.EditedChannelPost.From

	} else if update.EditedMessage != nil {
		user = update.EditedMessage.From

	} else if update.InlineQuery != nil {
		user = update.InlineQuery.From

	} else if update.Message != nil {
		user = update.Message.From

	} else if update.PreCheckoutQuery != nil {
		user = update.PreCheckoutQuery.From

	} else if update.ShippingQuery != nil {
		user = update.ShippingQuery.From
	} else {
		user = nil
	}

	// Check membership in native group
	status := membership[user.ID]
	var err error
	if statuses[status] == "" {

		if err := addNewUser(bot, user); err != nil {
			log.Panic(err)
		}

		if membership, err = loadAllUsersStatusFromBase(); err != nil {
			log.Panic(err)
		}

		return false

	} else if statuses[status] != "valid" {

		if membership, err = loadAllUsersStatusFromBase(); err != nil {
			log.Panic(err)
		}

		return false
	}

	return true
}
