package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"

	tgbotapi "github.com/skinass/telegram-bot-api/v5"
)

var (
	BotToken   = ""
	WebhookURL = ""
)

type Task struct {
	ID         int
	Title      string
	CreatorID  int64
	Creator    string
	AssigneeID *int64
	Assignee   string
	Done       bool
}

type TaskBot struct {
	bot    *tgbotapi.BotAPI
	tasks  []Task
	nextID int
	mu     sync.Mutex
}

func loadEnv() {
	data, err := os.ReadFile(".env")
	if err != nil {
		return
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}

func (b *TaskBot) send(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	_, err := b.bot.Send(msg)
	if err != nil {
		log.Printf("err: invalid send: %v", err)
	}
}

func (b *TaskBot) activateTasks() []Task {
	res := make([]Task, 0)
	for _, task := range b.tasks {
		if !task.Done {
			res = append(res, task)
		}
	}

	return res
}

func (b *TaskBot) cmdTasks(chatID int64, userID int64) {
	b.mu.Lock()
	activeTasks := b.activateTasks()
	b.mu.Unlock()

	if len(activeTasks) == 0 {
		msg := "Нет задач"
		b.send(chatID, msg)
		log.Println("Отправлено сообщение: " + msg)

		return
	}

	var sb strings.Builder
	for i, task := range activeTasks {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(fmt.Sprintf("%d. %s by @%s", task.ID, task.Title, task.Creator))

		if task.AssigneeID != nil && *task.AssigneeID == userID {
			sb.WriteString(fmt.Sprintf("\nassignee: я\n/unassign_%d /resolve_%d", task.ID, task.ID))
		} else if task.AssigneeID != nil {
			sb.WriteString(fmt.Sprintf("\nassignee: @%s", task.Assignee))
		} else {
			sb.WriteString(fmt.Sprintf("\n/assign_%d", task.ID))
		}
	}

	b.send(chatID, sb.String())
}

func (b *TaskBot) cmdTasksMine(chatID int64, userID int64) {
	b.mu.Lock()

	activeTasks := make([]Task, 0)
	for _, task := range b.tasks {
		if !task.Done && task.AssigneeID != nil && userID == *task.AssigneeID {
			activeTasks = append(activeTasks, task)
		}
	}
	b.mu.Unlock()

	if len(activeTasks) == 0 {
		msg := "Нет задач"
		b.send(chatID, msg)
		log.Println("Отправлено сообщение: " + msg)
		return
	}

	var sb strings.Builder
	for i, task := range activeTasks {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(fmt.Sprintf("%d. %s by @%s", task.ID, task.Title, task.Creator))

		if task.AssigneeID != nil && *task.AssigneeID == userID {
			sb.WriteString(fmt.Sprintf("\n/unassign_%d /resolve_%d", task.ID, task.ID))
		} else {
			sb.WriteString(fmt.Sprintf("\n/assign_%d", task.ID))
		}
	}

	b.send(chatID, sb.String())
	log.Println("Отправлено сообщение: " + sb.String())
}

func (b *TaskBot) cmdTasksOwner(chatID int64, userID int64) {
	b.mu.Lock()
	res := make([]Task, 0)

	for _, t := range b.tasks {
		if t.CreatorID == userID && !t.Done {
			res = append(res, t)
		}
	}
	b.mu.Unlock()

	if len(res) == 0 {
		b.send(chatID, "Нет задач")
		return
	}

	var sb strings.Builder
	for i, task := range res {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(fmt.Sprintf("%d. %s by @%s", task.ID, task.Title, task.Creator))

		if task.AssigneeID != nil && *task.AssigneeID == userID {
			sb.WriteString(fmt.Sprintf("\n/unassign_%d /resolve_%d", task.ID, task.ID))
		} else {
			sb.WriteString(fmt.Sprintf("\n/assign_%d", task.ID))
		}
	}

	b.send(chatID, sb.String())
	log.Println("Отправлено сообщение: " + sb.String())

}

func (b *TaskBot) cmdNew(chatID int64, userID int64, username string, text string) {
	title := strings.TrimPrefix(text, "/new ")
	title = strings.TrimSpace(title)

	if title == "" {
		log.Println("Передана пустая задача")
		msg := "Требуется непустая задача"
		b.send(chatID, msg)
		return
	}
	b.mu.Lock()
	task := Task{
		ID:        b.nextID,
		Title:     title,
		CreatorID: userID,
		Creator:   username,
	}
	b.nextID++
	b.tasks = append(b.tasks, task)

	b.mu.Unlock()

	b.send(chatID, fmt.Sprintf(`Задача "%s" создана, id=%d`, title, task.ID))

}

// Назначает задачу на себя.
// Уведомляет себя, автора (если не автор) и
// предыдущего исполнителя (если был другой).
func (b *TaskBot) cmdAssign(chatID int64, userID int64, username string, id int) {
	b.mu.Lock()

	var task *Task
	for i := range b.tasks {
		if b.tasks[i].ID == id && !b.tasks[i].Done {
			task = &b.tasks[i]
			break
		}
	}

	if task == nil {
		b.mu.Unlock()
		b.send(chatID, "Задача не найдена")
		return
	}

	title := task.Title
	creatorID := task.CreatorID
	oldAssigneeID := task.AssigneeID

	task.AssigneeID = &userID
	task.Assignee = username

	b.mu.Unlock()

	b.send(chatID, fmt.Sprintf(`Задача "%s" назначена на вас`, title))

	if oldAssigneeID != nil && *oldAssigneeID != userID {
		b.send(*oldAssigneeID, fmt.Sprintf(`Задача "%s" назначена на @%s`, title, username))
	}

	if oldAssigneeID == nil && creatorID != userID {
		b.send(creatorID, fmt.Sprintf(`Задача "%s" назначена на @%s`, title, username))
	}

}

func (b *TaskBot) cmdUnassign(chatID int64, userID int64, id int) {
	b.mu.Lock()

	var task *Task
	for i := range b.tasks {
		if b.tasks[i].ID == id && !b.tasks[i].Done {
			task = &b.tasks[i]
			break
		}
	}

	if task == nil {
		b.mu.Unlock()
		b.send(chatID, "Задача не найдена")
		return
	}

	if task.AssigneeID == nil || *task.AssigneeID != userID {
		b.mu.Unlock()
		b.send(chatID, "Задача не на вас")
		return
	}

	taskTitle := task.Title
	creatorID := task.CreatorID

	task.AssigneeID = nil
	task.Assignee = ""

	b.mu.Unlock()

	b.send(chatID, "Принято")

	if creatorID != userID {
		b.send(creatorID, fmt.Sprintf(`Задача "%s" осталась без исполнителя`, taskTitle))
	}
}

func (b *TaskBot) cmdResolve(chatID int64, userID int64, id int) {
	b.mu.Lock()

	var task *Task
	for i := range b.tasks {
		if b.tasks[i].ID == id && !b.tasks[i].Done {
			task = &b.tasks[i]
			break
		}
	}

	if task == nil {
		b.mu.Unlock()
		b.send(chatID, "Задача не найдена")
		return
	}

	if task.AssigneeID == nil || *task.AssigneeID != userID {
		b.mu.Unlock()
		b.send(chatID, "Задача назначена не на вас")
		return
	}

	creatorID := task.CreatorID
	taskTitle := task.Title
	assignee := task.Assignee

	task.Done = true
	b.mu.Unlock()

	b.send(chatID, fmt.Sprintf(`Задача "%s" выполнена`, taskTitle))

	if creatorID != userID {
		b.send(creatorID, fmt.Sprintf(`Задача "%s" выполнена @%s`, taskTitle, assignee))
	}

}

func (b *TaskBot) handleUpdate(update tgbotapi.Update) {
	if update.Message == nil {
		return
	}

	msg := update.Message
	chatID := msg.Chat.ID
	userID := msg.From.ID
	username := msg.From.UserName
	text := msg.Text

	if !strings.HasPrefix(text, "/") {
		return
	}

	switch text {
	case "/tasks":
		b.cmdTasks(chatID, userID)
	case "/my":
		b.cmdTasksMine(chatID, userID)
	case "/owner":
		b.cmdTasksOwner(chatID, userID)
	default:
		if strings.HasPrefix(text, "/new") {
			b.cmdNew(chatID, userID, username, text)
		} else if strings.HasPrefix(text, "/assign_") {
			id, err := strconv.Atoi(strings.TrimPrefix(text, "/assign_"))
			if err != nil {
				b.send(chatID, "Неверный ID задачи")
				return
			}
			b.cmdAssign(chatID, userID, username, id)
		} else if strings.HasPrefix(text, "/unassign_") {
			id, err := strconv.Atoi(strings.TrimPrefix(text, "/unassign_"))
			if err != nil {
				b.send(chatID, "Неверный ID задачи")
				return
			}
			b.cmdUnassign(chatID, userID, id)
		} else if strings.HasPrefix(text, "/resolve_") {
			id, err := strconv.Atoi(strings.TrimPrefix(text, "/resolve_"))
			if err != nil {
				b.send(chatID, "Неверный ID задачи")
				return
			}
			b.cmdResolve(chatID, userID, id)
		}
	}
}

func startTaskBot(ctx context.Context) error {
	loadEnv()

	BotToken := os.Getenv("TG_TOKEN")

	WebhookURL := os.Getenv("WEBHOOK_URL")

	bot, err := tgbotapi.NewBotAPI(BotToken)
	if err != nil {
		return err
	}

	bot.Debug = true
	fmt.Printf("Authorized on account %s\n", bot.Self.UserName)

	wh, err := tgbotapi.NewWebhook(WebhookURL)
	if err != nil {
		return err
	}
	if _, err := bot.Request(wh); err != nil {
		return err
	}

	updates := bot.ListenForWebhook("/")

	u, err := url.Parse(WebhookURL)
	if err != nil {
		return err
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}

	go func() {
		log.Fatalln("http err:", http.ListenAndServe(":"+port, nil))
	}()

	b := &TaskBot{bot: bot, nextID: 1}

	for {
		select {
		case <-ctx.Done():
			return nil
		case update, ok := <-updates:
			if !ok {
				return nil
			}
			b.handleUpdate(update)
		}
	}
}

func main() {
	err := startTaskBot(context.Background())
	if err != nil {
		panic(err)
	}
}
