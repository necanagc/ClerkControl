package telegram

import (
	"fmt"
	"strings"

	"taskbot/internal/task"
)

// renderList — единственное место, где формируется текст списка задач.
func renderList(tasks []task.Task, viewerID int64) string {
	if len(tasks) == 0 {
		return "Нет задач"
	}

	var sb strings.Builder
	for i, t := range tasks {
		if i > 0 {
			sb.WriteString("\n\n")
		}

		sb.WriteString(fmt.Sprintf("%d. %s by @%s", t.ID, t.Title, t.Creator))

		switch {
		case t.AssigneeID == nil:
			sb.WriteString(fmt.Sprintf("\n/assign_%d", t.ID))
		case t.AssignedTo(viewerID):
			sb.WriteString("\nassignee: я")
			sb.WriteString(fmt.Sprintf("\n/unassign_%d /resolve_%d", t.ID, t.ID))
		default:
			sb.WriteString(fmt.Sprintf("\nassignee: @%s", t.Assignee))
		}
	}

	return sb.String()
}

func taskCreated(t task.Task) string {
	return fmt.Sprintf(`Задача "%s" создана, id=%d`, t.Title, t.ID)
}

func taskAssigned(t task.Task) string {
	return fmt.Sprintf(`Задача "%s" назначена на вас`, t.Title)
}

func taskCompleted(t task.Task) string {
	return fmt.Sprintf(`Задача "%s" выполнена`, t.Title)
}
