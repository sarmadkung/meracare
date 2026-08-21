package server

import (
	"github.com/meracare/api/internal/appointments"
	"github.com/meracare/api/internal/careevents"
	"github.com/meracare/api/internal/config"
	"github.com/meracare/api/internal/medications"
	"github.com/meracare/api/internal/notifications"
	"github.com/meracare/api/internal/relationships"
	"github.com/meracare/api/internal/seniors"
	"github.com/meracare/api/internal/tasks"
)

// NewNotificationScheduler wires the one background process MeraCare runs.
//
// Built separately from New rather than returned by it, because the two have
// different lifetimes: the router is a value the HTTP server holds, and the
// scheduler is something the process starts and must wait for on the way out.
// Bundling them would make every test that wants a router also have to think
// about stopping a goroutine (plans/phase11.md §36).
//
// Its collaborators are built here from the same pool. Repositories are handles
// over that pool rather than stateful objects, so a second one costs nothing and
// keeps this function readable as a list of what the scheduler needs.
func NewNotificationScheduler(deps Dependencies) *notifications.Scheduler {
	repository := notifications.NewRepository(deps.Pool)
	seniorRepo := seniors.NewRepository(deps.Pool)
	eventRepo := careevents.NewRepository(deps.Pool)

	relationshipRepo := relationships.NewRepository(deps.Pool)
	recorder := careevents.NewRecorder(deps.Pool, eventRepo)

	taskService := tasks.NewService(
		tasks.NewRepository(deps.Pool), seniorRepo, relationshipRepo, recorder)
	medicationService := medications.NewService(
		medications.NewRepository(deps.Pool), seniorRepo, recorder)
	appointmentService := appointments.NewService(
		appointments.NewRepository(deps.Pool), seniorRepo, relationshipRepo, recorder)

	return notifications.NewScheduler(notifications.SchedulerDependencies{
		Repository: repository,
		Sender:     newPushSender(deps.Config),
		Roster:     roster{repo: seniorRepo},
		// The same three sources the reminder plan reads, so a dose reminds at
		// the same moment whether the device scheduled it or the server pushed
		// it. One expansion of a recurrence rule, consumed twice
		// (plans/phase8.md §21).
		Reminders: map[notifications.Type]notifications.ScheduleSource{
			notifications.TypeTaskReminder:        taskSource{service: taskService},
			notifications.TypeMedicationReminder:  medicationSource{service: medicationService},
			notifications.TypeAppointmentReminder: appointmentSource{service: appointmentService},
		},
		Overdue:  overdueSource{service: taskService},
		Activity: activitySource{repo: eventRepo},
		Logger:   deps.Logger,
	}, notifications.SchedulerOptions{
		Interval:  deps.Config.NotificationSchedulerInterval,
		Retention: deps.Config.NotificationRetention,
	})
}

// newPushSender chooses the push provider.
//
// Disabled is the default and the honest one: MeraCare holds no push
// credentials, so there is nowhere to send a notification and saying so is
// better than pretending. The inbox works either way, which is why this is a
// configuration switch rather than a startup failure (plans/phase11.md §43).
func newPushSender(cfg *config.Config) notifications.PushSender {
	if !cfg.PushEnabled {
		return notifications.DisabledSender{}
	}
	return notifications.NewExpoSender(notifications.ExpoSenderOptions{
		AccessToken: cfg.ExpoAccessToken,
	})
}
