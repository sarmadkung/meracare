// Package server wires the modular monolith together: one router, one process,
// modules mounted side by side (docs/05-api-and-backend-spec.md).
package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/genxcare/api/internal/appointments"
	"github.com/genxcare/api/internal/auth"
	"github.com/genxcare/api/internal/authz"
	"github.com/genxcare/api/internal/careevents"
	"github.com/genxcare/api/internal/config"
	"github.com/genxcare/api/internal/database"
	"github.com/genxcare/api/internal/invitations"
	"github.com/genxcare/api/internal/medications"
	"github.com/genxcare/api/internal/members"
	"github.com/genxcare/api/internal/messages"
	"github.com/genxcare/api/internal/notes"
	"github.com/genxcare/api/internal/notifications"
	"github.com/genxcare/api/internal/relationships"
	"github.com/genxcare/api/internal/seniors"
	"github.com/genxcare/api/internal/summary"
	"github.com/genxcare/api/internal/tasks"
	"github.com/genxcare/api/internal/users"
	"github.com/genxcare/api/pkg/httpx"
)

// Dependencies are the collaborators the router needs.
type Dependencies struct {
	Config   *config.Config
	Logger   *slog.Logger
	Pool     *database.Pool
	Verifier auth.Verifier
}

// New builds the fully wired HTTP handler.
func New(deps Dependencies) http.Handler {
	userRepo := users.NewRepository(deps.Pool)
	userService := users.NewService(userRepo)
	userHandler := users.NewHandler(userService)

	relationshipRepo := relationships.NewRepository(deps.Pool)
	// Every senior-scoped route resolves access through this guard.
	guard := authz.NewGuard(relationshipRepo)

	// Every domain that changes something records what happened through this,
	// in the same transaction as the change (plans/phase7.md §§6, 23, 26).
	eventRepo := careevents.NewRepository(deps.Pool)
	recorder := careevents.NewRecorder(deps.Pool, eventRepo)
	activityHandler := careevents.NewHandler(careevents.NewService(eventRepo))

	seniorRepo := seniors.NewRepository(deps.Pool)
	seniorHandler := seniors.NewHandler(seniors.NewService(seniorRepo, relationshipRepo), guard)

	memberHandler := members.NewHandler(members.NewService(relationshipRepo, recorder), guard)

	requireAuth := auth.RequireAuth(deps.Verifier, userService)
	invitationService := invitations.NewService(
		invitations.NewRepository(deps.Pool),
		relationshipRepo,
		userLookup{repo: userRepo},
		seniorRepo,
		recorder,
	)
	invitationHandler := invitations.NewHandler(invitationService, guard, requireAuth)

	taskService := tasks.NewService(tasks.NewRepository(deps.Pool), seniorRepo, relationshipRepo, recorder)
	taskHandler := tasks.NewHandler(taskService, guard)

	medicationService := medications.NewService(medications.NewRepository(deps.Pool), seniorRepo, recorder)
	medicationHandler := medications.NewHandler(medicationService, guard)

	appointmentService := appointments.NewService(
		appointments.NewRepository(deps.Pool), seniorRepo, relationshipRepo, recorder,
	)
	appointmentHandler := appointments.NewHandler(appointmentService, guard)

	// Composes the domains above so the care circle can be summarised in one
	// request rather than two per senior.
	summaryHandler := summary.NewHandler(summary.NewService(
		seniors.NewService(seniorRepo, relationshipRepo), taskService, medicationService,
	))
	todayHandler := summary.NewTodayHandler(summary.NewTodayService(
		seniors.NewService(seniorRepo, relationshipRepo),
		taskService, medicationService, appointmentService,
	))
	noteHandler := notes.NewHandler(notes.NewService(notes.NewRepository(deps.Pool), recorder), guard)
	messageHandler := messages.NewHandler(messages.NewService(messages.NewRepository(deps.Pool)), guard)

	// Reminders read the three domains above through narrow adapters, so the
	// notification code never learns what a dose is (plans/phase8.md §1).
	notificationHandler := notifications.NewHandler(notifications.NewService(
		notifications.NewRepository(deps.Pool),
		circleSource{repo: seniorRepo},
		taskSource{service: taskService},
		medicationSource{service: medicationService},
		appointmentSource{service: appointmentService},
	))

	router := chi.NewRouter()
	router.NotFound(httpx.NotFoundHandler())
	router.MethodNotAllowed(httpx.MethodNotAllowedHandler())

	router.Use(middleware.RequestID)
	router.Use(httpx.RequestLogger(deps.Logger))
	router.Use(httpx.Recoverer)
	router.Use(httpx.CORS(deps.Config.CORSAllowedOrigins))
	router.Use(middleware.Timeout(deps.Config.RequestTimeout))

	// Unauthenticated operational endpoints.
	router.Get("/healthz", healthHandler)
	router.Get("/readyz", readyHandler(deps.Pool))

	// Invitations are mounted outside the authenticated group: previewing an
	// invitation must work before the recipient has an account. The routes that
	// need a signed-in user apply the middleware themselves.
	router.Route("/v1/invitations", func(tokens chi.Router) {
		tokens.Mount("/", invitationHandler.TokenRoutes())
	})

	router.Route("/v1", func(v1 chi.Router) {
		v1.Use(requireAuth)
		v1.Mount("/me", userHandler.Routes())
		v1.Mount("/notifications", notificationHandler.Routes())
		v1.Mount("/today", todayHandler.Routes())
		v1.Mount("/tasks", taskHandler.TaskRoutes())
		v1.Mount("/medications", medicationHandler.MedicationRoutes())
		v1.Mount("/appointments", appointmentHandler.AppointmentRoutes())
		v1.Mount("/notes", noteHandler.NoteRoutes())
		v1.Mount("/seniors", seniorHandler.Routes(seniors.SubRoutes{
			Summary:      summaryHandler.Routes(),
			Members:      memberHandler.Routes(),
			Invitations:  invitationHandler.SeniorRoutes(),
			Tasks:        taskHandler.SeniorRoutes(),
			Medications:  medicationHandler.SeniorRoutes(),
			Appointments: appointmentHandler.SeniorRoutes(),
			Activity:     activityHandler.SeniorRoutes(guard),
			Notes:        noteHandler.SeniorRoutes(),
			Messages:     messageHandler.SeniorRoutes(),
		}))
	})

	return router
}

// healthHandler reports that the process is running. It performs no dependency
// checks so a liveness probe never restarts the API over a database blip.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, r, http.StatusOK, map[string]string{"status": "ok"})
}

// readyHandler reports whether the API can serve traffic, i.e. the database is
// reachable.
func readyHandler(pool *database.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := database.Ping(ctx, pool); err != nil {
			httpx.WriteError(w, r, httpx.ErrUnavailable("The service is not ready.").WithCause(err))
			return
		}
		httpx.WriteJSON(w, r, http.StatusOK, map[string]string{"status": "ready"})
	}
}
