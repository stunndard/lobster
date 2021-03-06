package support

import "github.com/LunaNode/lobster"

import "log"
import "time"

type TicketMessage struct {
	Id      int
	Staff   bool
	Message string
	Time    time.Time
}

type Ticket struct {
	Id         int
	UserId     int
	Name       string
	Status     string
	Time       time.Time
	ModifyTime time.Time
	Messages   []*TicketMessage
}

func TicketList(userId int) []*Ticket {
	return ticketListHelper(
		db.Query(
			"SELECT id, user_id, name, status, time, modify_time "+
				"FROM tickets "+
				"WHERE user_id = ? ORDER BY modify_time DESC",
			userId,
		),
	)
}

func TicketListActive(userId int) []*Ticket {
	return ticketListHelper(
		db.Query(
			"SELECT id, user_id, name, status, time, modify_time "+
				"FROM tickets "+
				"WHERE user_id = ? AND (status = 'open' OR status = 'answered') "+
				"ORDER BY modify_time DESC",
			userId,
		),
	)
}

func TicketListAll() []*Ticket {
	return ticketListHelper(
		db.Query(
			"SELECT id, user_id, name, status, time, modify_time " +
				"FROM tickets " +
				"ORDER BY FIELD(status, 'open', 'answered', 'closed'), modify_time DESC",
		),
	)
}

func ticketListHelper(rows lobster.Rows) []*Ticket {
	tickets := make([]*Ticket, 0)
	defer rows.Close()
	for rows.Next() {
		ticket := Ticket{}
		rows.Scan(&ticket.Id, &ticket.UserId, &ticket.Name, &ticket.Status, &ticket.Time, &ticket.ModifyTime)
		tickets = append(tickets, &ticket)
	}
	return tickets
}

func TicketDetails(userId int, ticketId int, staff bool) *Ticket {
	var rows lobster.Rows
	if staff {
		rows = db.Query("SELECT id, user_id, name, status, time, modify_time FROM tickets WHERE id = ?", ticketId)
	} else {
		rows = db.Query("SELECT id, user_id, name, status, time, modify_time FROM tickets WHERE user_id = ? AND id = ?", userId, ticketId)
	}
	tickets := ticketListHelper(rows)
	if len(tickets) != 1 {
		return nil
	}
	ticket := tickets[0]

	rows = db.Query("SELECT id, staff, message, time FROM ticket_messages WHERE ticket_id = ? ORDER BY id", ticketId)
	defer rows.Close()
	for rows.Next() {
		message := &TicketMessage{}
		rows.Scan(&message.Id, &message.Staff, &message.Message, &message.Time)
		ticket.Messages = append(ticket.Messages, message)
	}

	return ticket
}

func ticketOpen(userId int, name string, message string, staff bool) (int, error) {
	if name == "" || message == "" {
		return 0, L.Error("subject_message_empty")
	} else if len(message) > 16384 {
		return 0, L.Errorf("message_too_long", "15,000")
	}

	user := lobster.UserDetails(userId)
	if !staff && (user == nil || user.Status == "new") {
		return 0, L.Errorf("ticket_for_support", cfg.Default.AdminEmail)
	}

	result := db.Exec("INSERT INTO tickets (user_id, name, status, modify_time) VALUES (?, ?, 'open', NOW())", userId, name)
	ticketId := result.LastInsertId()
	db.Exec("INSERT INTO ticket_messages (ticket_id, staff, message) VALUES (?, ?, ?)", ticketId, staff, message)
	if staff {
		lobster.MailWrap(userId, "ticketOpen", TicketUpdateEmail{Id: ticketId, Subject: name, Message: message}, false)
	} else {
		lobster.MailWrap(-1, "ticketOpen", TicketUpdateEmail{Id: ticketId, Subject: name, Message: message}, false)
	}
	log.Printf("Ticket opened for user %d: %s", userId, name)
	return ticketId, nil
}

func ticketReply(userId int, ticketId int, message string, staff bool) error {
	if message == "" {
		return L.Error("message_empty")
	}

	ticket := TicketDetails(userId, ticketId, staff)
	if ticket == nil {
		return L.Error("invalid_ticket")
	}

	db.Exec("INSERT INTO ticket_messages (ticket_id, staff, message) VALUES (?, ?, ?)", ticketId, staff, message)

	// update ticket status
	newStatus := "open"
	if staff {
		newStatus = "answered"
		lobster.MailWrap(userId, "ticketReply", TicketUpdateEmail{Id: ticketId, Subject: ticket.Name, Message: message}, false)
	} else {
		lobster.MailWrap(-1, "ticketReply", TicketUpdateEmail{Id: ticketId, Subject: ticket.Name, Message: message}, false)
	}
	db.Exec("UPDATE tickets SET modify_time = NOW(), status = ? WHERE id = ?", newStatus, ticketId)
	log.Printf("Ticket reply for user %d on ticket #%d %s", userId, ticketId, ticket.Name)
	return nil
}

func ticketClose(userId int, ticketId int) {
	db.Exec("UPDATE tickets SET modify_time = NOW(), status = 'closed' WHERE id = ? AND user_id = ?", ticketId, userId)
}
