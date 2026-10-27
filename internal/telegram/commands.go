package telegram

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	tgbotapi "github.com/skinass/telegram-bot-api/v5"

	"taskbot/internal/task"
	"taskbot/internal/user"
)

// cmdStart показывает приветствие с кнопками.
func (b *Bot) cmdStart(chatID int64, name string) {
	b.sendWithKeyboard(chatID, welcomeText(name), mainMenu())
}

// cmdHelp показывает справку.
func (b *Bot) cmdHelp(chatID int64) {
	b.sendWithKeyboard(chatID, helpText(), backKeyboard())
}

// cmdTasks показывает все открытые задачи чата.
func (b *Bot) cmdTasks(ctx context.Context, chatID, userID int64) {
	b.showList(ctx, chatID, 0, userID, filterAll, 0)
}

// cmdTasksMine показывает задачи, назначенные на пользователя.
func (b *Bot) cmdTasksMine(ctx context.Context, chatID, userID int64) {
	b.showList(ctx, chatID, 0, userID, filterMine, 0)
}

// cmdTasksOwner показывает задачи, созданные пользователем.
func (b *Bot) cmdTasksOwner(ctx context.Context, chatID, userID int64) {
	b.showList(ctx, chatID, 0, userID, filterOwner, 0)
}

// cmdNew создаёт задачу из остатка сообщения.
func (b *Bot) cmdNew(ctx context.Context, chatID, userID int64, username, title string) {
	b.createTask(ctx, chatID, userID, username, title)
}

// promptNew просит прислать название задачи ответом на сообщение.
func (b *Bot) promptNew(chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "📝 Напишите название задачи одним сообщением.\n\nОтменить: /cancel")
	msg.ReplyMarkup = tgbotapi.ForceReply{ForceReply: true}

	sent, err := b.api.Send(msg)
	if err != nil {
		b.log.Error("prompt new task", "chat_id", chatID, "err", err)
		return
	}

	b.setPending(chatID, sent.MessageID)
}

// createTask — общий путь создания задачи для команды и кнопки.
func (b *Bot) createTask(ctx context.Context, chatID, userID int64, username, title string) {
	t, err := b.svc.Create(ctx, task.CreateInput{
		ChatID:    chatID,
		Title:     title,
		CreatorID: userID,
		Creator:   username,
	})
	if err != nil {
		if errors.Is(err, task.ErrEmptyTitle) {
			b.send(chatID, "🤔 Нужен текст задачи.\nНапример: /new Купить хлеб")
			return
		}
		b.log.Error("create task", "chat_id", chatID, "err", err)
		b.send(chatID, "😔 Не удалось создать задачу, попробуйте ещё раз")
		return
	}

	b.sendWithKeyboard(chatID, taskCreatedText(t), createdKeyboard(t))
}

// cmdAssign назначает задачу: на себя, на ответившего в реплае или на @username.
func (b *Bot) cmdAssign(ctx context.Context, chatID, actorID int64, actorName string, id int, reply *tgbotapi.User, usernameRef string) {
	targetID, targetName := actorID, actorName

	switch {
	case reply != nil:
		targetID, targetName = reply.ID, reply.UserName
	case usernameRef != "":
		u, err := b.users.ByUsername(ctx, usernameRef)
		if errors.Is(err, user.ErrNotFound) {
			b.send(chatID, fmt.Sprintf("🤷 Не знаю пользователя @%s.\nОн должен хотя бы раз написать боту.", usernameRef))
			return
		}
		if err != nil {
			b.log.Error("find user", "username", usernameRef, "err", err)
			b.send(chatID, "😔 Не получилось найти пользователя")
			return
		}
		targetID, targetName = u.ID, u.Username
	}

	t, err := b.svc.AssignTo(ctx, chatID, id, targetID, targetName, actorID, actorName)
	if err != nil {
		b.reportCommandError(chatID, "assign", err, "Эта задача не на вас")
		return
	}

	if targetID == actorID {
		b.sendWithKeyboard(chatID, taskAssignedText(t), backKeyboard())
		return
	}

	b.sendWithKeyboard(chatID, taskAssignedToText(t), backKeyboard())
}

// cmdUnassign снимает задачу с себя.
func (b *Bot) cmdUnassign(ctx context.Context, chatID, userID int64, id int) {
	t, err := b.svc.Unassign(ctx, chatID, id, userID)
	if err != nil {
		b.reportCommandError(chatID, "unassign", err, "Эта задача не на вас")
		return
	}

	b.sendWithKeyboard(chatID, taskUnassignedText(t), backKeyboard())
}

// cmdResolve закрывает задачу.
func (b *Bot) cmdResolve(ctx context.Context, chatID, userID int64, username string, id int) {
	t, err := b.svc.Complete(ctx, chatID, id, userID, username)
	if err != nil {
		b.reportCommandError(chatID, "resolve", err, "Эта задача не на вас")
		return
	}

	b.sendWithKeyboard(chatID, taskCompletedText(t), backKeyboard())
}

// reportCommandError переводит ошибки сервиса в человеческие тексты.
func (b *Bot) reportCommandError(chatID int64, op string, err error, forbiddenText string) {
	switch {
	case errors.Is(err, task.ErrNotFound):
		b.send(chatID, "🤷 Задача не найдена — возможно, её уже закрыли")
	case errors.Is(err, task.ErrForbidden):
		b.send(chatID, forbiddenText)
	default:
		b.log.Error("command failed", "op", op, "chat_id", chatID, "err", err)
		b.send(chatID, "😔 Не получилось, попробуйте ещё раз")
	}
}

func parseTaskID(raw string) (int, bool) {
	id, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}

	return id, true
}
