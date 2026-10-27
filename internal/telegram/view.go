package telegram

import (
	"fmt"
	"strings"

	tgbotapi "github.com/skinass/telegram-bot-api/v5"

	"taskbot/internal/task"
)

// pageSize — сколько задач показывать на одной странице.
const pageSize = 5

// Коды фильтров: короткие, чтобы влезали в callback_data.
const (
	filterAll   = "a"
	filterMine  = "m"
	filterOwner = "o"
)

func filterTitle(code string) string {
	switch code {
	case filterMine:
		return "🙋 Мои задачи"
	case filterOwner:
		return "✍️ Созданные мной"
	default:
		return "📋 Открытые задачи"
	}
}

// welcomeText — первое сообщение пользователю.
func welcomeText(name string) string {
	if name == "" {
		name = "друг"
	}

	return fmt.Sprintf(`👋 Привет, %s!

Я веду задачи прямо в чате: создавайте, берите на себя и закрывайте — без таблиц и досок.

Создать задачу: /new Купить хлеб
Или выберите действие кнопками ниже 👇`, name)
}

// helpText — справка. Единственное место, где перечислены команды.
func helpText() string {
	return `ℹ️ Что я умею

/new <текст> — создать задачу
/tasks — все открытые задачи
/my — задачи на мне
/owner — созданные мной
/help — эта справка

Кнопки под задачами:
✋ взять на себя · ✅ закрыть · ↩️ снять`
}

// emptyText — что показать, если список пуст. Зависит от выбранного фильтра.
func emptyText(filter string) string {
	switch filter {
	case filterMine:
		return "🙋 На вас пока ничего не назначено.\nПосмотрите свободные задачи: /tasks"
	case filterOwner:
		return "✍️ Вы ещё не создавали задач.\nНачните с /new Купить хлеб"
	default:
		return "✨ Открытых задач нет.\nДобавьте первую: /new Купить хлеб"
	}
}

// renderList собирает текст списка задач.
func renderList(tasks []task.Task, viewerID int64, filter string, page, pages int) string {
	if len(tasks) == 0 {
		return emptyText(filter)
	}

	var sb strings.Builder
	sb.WriteString(filterTitle(filter))

	for _, t := range tasks {
		sb.WriteString("\n\n")
		sb.WriteString(fmt.Sprintf("#%d %s", t.ID, t.Title))
		sb.WriteString("\n")
		sb.WriteString(statusLine(t, viewerID))
	}

	if pages > 1 {
		sb.WriteString(fmt.Sprintf("\n\nСтраница %d из %d", page+1, pages))
	}

	return sb.String()
}

// statusLine описывает автора и исполнителя задачи.
func statusLine(t task.Task, viewerID int64) string {
	line := "👤 @" + t.Creator

	switch {
	case t.AssigneeID == nil:
		line += " · свободна"
	case t.AssignedTo(viewerID):
		line += " · 🙋 вы"
	default:
		line += " · 🙋 @" + t.Assignee
	}

	return line
}

func taskCreatedText(t task.Task) string {
	return fmt.Sprintf("✅ Задача #%d создана\n\n«%s»", t.ID, t.Title)
}

func taskAssignedText(t task.Task) string {
	return fmt.Sprintf("✋ Задача #%d теперь на вас\n\n«%s»", t.ID, t.Title)
}

func taskAssignedToText(t task.Task) string {
	return fmt.Sprintf("👤 Задача #%d назначена на @%s\n\n«%s»", t.ID, t.Assignee, t.Title)
}

func taskUnassignedText(t task.Task) string {
	return fmt.Sprintf("↩️ Задача #%d снова свободна\n\n«%s»", t.ID, t.Title)
}

func taskCompletedText(t task.Task) string {
	return fmt.Sprintf("🎉 Задача #%d закрыта\n\n«%s»", t.ID, t.Title)
}

// mainMenu — клавиатура приветствия.
func mainMenu() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🙋 Мои задачи", "ls:"+filterMine+":0"),
			tgbotapi.NewInlineKeyboardButtonData("📋 Все задачи", "ls:"+filterAll+":0"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ Новая задача", "new"),
			tgbotapi.NewInlineKeyboardButtonData("ℹ️ Помощь", "hlp"),
		),
	)
}

// backKeyboard — кнопки возврата к списку.
func backKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 К задачам", "ls:"+filterAll+":0"),
			tgbotapi.NewInlineKeyboardButtonData("🙋 Мои", "ls:"+filterMine+":0"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ Новая задача", "new"),
		),
	)
}

// listKeyboard собирает кнопки под списком: действия по задачам и пагинацию.
func listKeyboard(tasks []task.Task, viewerID int64, filter string, page, pages int) tgbotapi.InlineKeyboardMarkup {
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(tasks)+1)

	for _, t := range tasks {
		rows = append(rows, taskRow(t, viewerID, filter, page))
	}

	if pages > 1 {
		rows = append(rows, paginationRow(filter, page, pages))
	}

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func taskRow(t task.Task, viewerID int64, filter string, page int) []tgbotapi.InlineKeyboardButton {
	// Источник нужен, чтобы после действия перерисовать тот же список и страницу.
	source := fmt.Sprintf(":%s:%d", filter, page)

	if t.AssignedTo(viewerID) {
		return tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("✅ Готово #%d", t.ID), fmt.Sprintf("don:%d%s", t.ID, source)),
			tgbotapi.NewInlineKeyboardButtonData("↩️ Снять", fmt.Sprintf("uns:%d%s", t.ID, source)),
		)
	}

	return tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("✋ Взять #%d", t.ID), fmt.Sprintf("asg:%d%s", t.ID, source)),
	)
}

func paginationRow(filter string, page, pages int) []tgbotapi.InlineKeyboardButton {
	prev, next := page-1, page+1

	prevData := "noop"
	if prev >= 0 {
		prevData = fmt.Sprintf("ls:%s:%d", filter, prev)
	}

	nextData := "noop"
	if next < pages {
		nextData = fmt.Sprintf("ls:%s:%d", filter, next)
	}

	return tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("◀️", prevData),
		tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%d/%d", page+1, pages), "noop"),
		tgbotapi.NewInlineKeyboardButtonData("▶️", nextData),
	)
}

// createdKeyboard — кнопки под только что созданной задачей.
func createdKeyboard(t task.Task) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✋ Взять на себя", fmt.Sprintf("asg:%d:%s:0", t.ID, filterAll)),
			tgbotapi.NewInlineKeyboardButtonData("📋 К списку", "ls:"+filterAll+":0"),
		),
	)
}
