package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/palchrb/inreach-project/internal/command"
	"github.com/palchrb/inreach-project/internal/config"
	"github.com/palchrb/inreach-project/internal/decoder"
	gm "github.com/palchrb/inreach-project/internal/hermes"
	"github.com/palchrb/inreach-project/internal/store"
)

// Service is the core inReach assistant service.
type Service struct {
	cfg       *config.Config
	auth      *gm.HermesAuth
	api       *gm.HermesAPI
	sr        *gm.HermesSignalR
	router    *Router
	responder *Responder
	logger    *slog.Logger
	selfID    string
	history   *store.ChatHistory
}

// New creates a new Service.
func New(cfg *config.Config, logger *slog.Logger) *Service {
	auth := gm.NewHermesAuth(
		gm.WithSessionDir(cfg.Garmin.SessionDir),
		gm.WithLogger(logger),
	)

	api := gm.NewHermesAPI(auth, gm.WithAPILogger(logger))
	sr := gm.NewHermesSignalR(auth, gm.WithSignalRLogger(logger))

	selfID := gm.PhoneToHermesUserID(cfg.Garmin.Phone)

	// Initialize stores
	history := store.NewChatHistory("data/chat_history.json", 60*time.Minute)
	shelterState := store.NewShelterState()

	// Initialize handlers
	handlers := RouterHandlers{
		ChatGPT:   command.NewChatGPTHandler(cfg.APIKeys.OpenAI, cfg.APIKeys.OpenAIModel, history),
		Weather:   command.NewWeatherHandler(cfg.APIKeys.OpenAI, cfg.APIKeys.OpenAIModel, cfg.APIKeys.TimezoneDB),
		Avalanche: command.NewAvalancheHandler(),
		Shelter:   command.NewShelterHandler(shelterState, "data/cabins.json"),
		Route:     command.NewRouteHandler(cfg.APIKeys.OpenRouteService, shelterState),
		Train:     command.NewTrainHandler(),
		MapShare:  command.NewMapShareHandler(),
	}

	router := NewRouter(handlers)
	responder := NewResponder(api, cfg.CharLimit, logger)

	return &Service{
		cfg:       cfg,
		auth:      auth,
		api:       api,
		sr:        sr,
		router:    router,
		responder: responder,
		logger:    logger,
		selfID:    selfID,
		history:   history,
	}
}

// Auth returns the HermesAuth for login flow.
func (s *Service) Auth() *gm.HermesAuth {
	return s.auth
}

// Resume restores the Garmin session from disk.
func (s *Service) Resume(ctx context.Context) error {
	return s.auth.Resume(ctx)
}

// Run starts the service and blocks until ctx is cancelled.
func (s *Service) Run(ctx context.Context) error {
	// Auto-refresh cabin cache if missing or older than 7 days
	s.refreshCabinCacheIfNeeded()

	// Validate session with a lightweight API call
	_, err := s.api.GetNetworkProperties(ctx)
	if err != nil {
		return fmt.Errorf("session validation failed: %w", err)
	}
	s.logger.Info("Session validated", "phone", s.cfg.Garmin.Phone)

	// Start decoder web UI if enabled
	if s.cfg.Decoder.Enabled {
		go func() {
			s.logger.Info("Starting decoder web UI", "listen", s.cfg.Decoder.Listen)
			srv := &http.Server{Addr: s.cfg.Decoder.Listen, Handler: decoder.Handler()}
			go func() {
				<-ctx.Done()
				srv.Close()
			}()
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				s.logger.Error("Decoder web UI error", "error", err)
			}
		}()
	}

	// Register message handler
	s.sr.OnMessage(func(msg gm.MessageModel) {
		s.handleMessage(ctx, msg)
	})

	s.sr.OnOpen(func() {
		s.logger.Info("Connected to Garmin Messenger")
	})

	s.sr.OnClose(func() {
		s.logger.Warn("Disconnected from Garmin Messenger")
	})

	s.sr.OnError(func(err error) {
		s.logger.Error("SignalR error", "error", err)
	})

	s.logger.Info("Starting SignalR connection...")
	return s.sr.Start(ctx)
}

// Stop disconnects the service.
func (s *Service) Stop() {
	s.sr.Stop()
}

func (s *Service) handleMessage(ctx context.Context, msg gm.MessageModel) {
	// Skip own messages
	if msg.From != nil && *msg.From == s.selfID {
		return
	}

	// Skip reactions
	if msg.MessageType != nil && msg.MessageType.IsReaction() {
		return
	}

	// Mark as delivered
	s.sr.MarkAsDelivered(msg.ConversationID, msg.MessageID)

	// Get message body
	body := ""
	if msg.MessageBody != nil {
		body = strings.TrimSpace(*msg.MessageBody)
	}
	if body == "" {
		s.logger.Debug("Skipping empty message", "messageId", msg.MessageID)
		return
	}

	// Extract coordinates
	var lat, lon, elev *float64
	if msg.UserLocation != nil {
		lat = msg.UserLocation.LatitudeDegrees
		lon = msg.UserLocation.LongitudeDegrees
		elev = msg.UserLocation.ElevationMeters
	}

	s.logger.Info("Received message",
		"messageId", msg.MessageID,
		"from", msg.From,
		"body", body,
		"hasLocation", lat != nil,
	)

	// Route to handler
	handler, args := s.router.Match(body)
	s.logger.Info("Routed to handler", "handler", handler.Name(), "args", args)

	// Determine char limit
	charLimit := s.cfg.CharLimit
	if handler.Name() == "chatgpt" {
		charLimit = s.cfg.ChatGPTCharLimit()
	}

	cc := &command.CommandContext{
		Ctx:       ctx,
		Message:   msg,
		Args:      args,
		Lat:       lat,
		Lon:       lon,
		Elevation: elev,
		CharLimit: charLimit,
		Logger:    s.logger.With("handler", handler.Name()),
	}

	// Execute handler
	parts, err := handler.Handle(cc)
	if err != nil {
		s.logger.Error("Handler error", "handler", handler.Name(), "error", err)
		parts = []string{"Feil ved behandling av forespørsel."}
	}

	// Send response
	if err := s.responder.Send(ctx, msg, parts); err != nil {
		s.logger.Error("Failed to send response", "error", err)
	}
}

const cabinsPath = "data/cabins.json"
const cabinsMaxAge = 7 * 24 * time.Hour

func (s *Service) refreshCabinCacheIfNeeded() {
	info, err := os.Stat(cabinsPath)
	if err == nil && time.Since(info.ModTime()) < cabinsMaxAge {
		s.logger.Debug("Cabin cache is fresh", "age", time.Since(info.ModTime()).Round(time.Hour))
		return
	}

	if err != nil {
		s.logger.Info("No cabin cache found, fetching from UT.no...")
	} else {
		s.logger.Info("Cabin cache is stale, refreshing from UT.no...", "age", time.Since(info.ModTime()).Round(time.Hour))
	}

	os.MkdirAll("data", 0o755)
	if err := command.FetchAndCacheCabins(s.logger, cabinsPath); err != nil {
		s.logger.Warn("Failed to refresh cabin cache, continuing with existing data", "error", err)
	}
}
