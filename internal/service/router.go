package service

import (
	"regexp"
	"strings"

	"github.com/palchrb/inreach-project/internal/command"
)

// route maps a regex pattern to a handler and a function to extract args.
type route struct {
	pattern *regexp.Regexp
	handler command.Handler
	argFunc func(msg string, matches []string) string
}

// Router matches incoming messages to command handlers.
type Router struct {
	routes  []route
	fallback command.Handler
}

// NewRouter creates a router with the given handlers.
func NewRouter(handlers RouterHandlers) *Router {
	r := &Router{fallback: handlers.ChatGPT}

	// Order matches the original Kode.js if/else chain
	if handlers.MapShare != nil {
		r.addRoute(`(?i)^locate\s+(\w+)`, handlers.MapShare, func(msg string, m []string) string {
			return m[1]
		})
	}
	if handlers.Shelter != nil {
		r.addRoute(`(?i)^shelter`, handlers.Shelter, func(msg string, m []string) string { return "" })
	}
	if handlers.Weather != nil {
		r.addRoute(`(?i)^vær`, handlers.Weather, func(msg string, m []string) string {
			// Pass the full message so handler can check for "detaljert", "i morgen" etc.
			return strings.TrimSpace(regexp.MustCompile(`(?i)^vær\s*`).ReplaceAllString(msg, ""))
		})
	}
	if handlers.Train != nil {
		r.addRoute(`(?i)^train\s+stationboard`, handlers.Train, func(msg string, m []string) string {
			return strings.TrimSpace(regexp.MustCompile(`(?i)^train\s+`).ReplaceAllString(msg, ""))
		})
		r.addRoute(`(?i)^train`, handlers.Train, func(msg string, m []string) string {
			return strings.TrimSpace(regexp.MustCompile(`(?i)^train\s*`).ReplaceAllString(msg, ""))
		})
	}
	if handlers.Avalanche != nil {
		r.addRoute(`(?i)^skred`, handlers.Avalanche, func(msg string, m []string) string { return "" })
	}
	if handlers.Route != nil {
		// Route to coordinates: "route lat,lon"
		r.addRoute(`(?i)^route\s+([-+]?\d{1,2}\.\d+),\s*([-+]?\d{1,3}\.\d+)`, handlers.Route, func(msg string, m []string) string {
			return m[1] + "," + m[2]
		})
		// Route to shelter number: "route N"
		r.addRoute(`(?i)^route\s+(\d+)`, handlers.Route, func(msg string, m []string) string {
			return m[1]
		})
	}

	return r
}

// RouterHandlers holds all available command handlers.
type RouterHandlers struct {
	MapShare  command.Handler
	Shelter   command.Handler
	Weather   command.Handler
	Train     command.Handler
	Avalanche command.Handler
	Route     command.Handler
	ChatGPT   command.Handler
}

func (r *Router) addRoute(pattern string, handler command.Handler, argFunc func(string, []string) string) {
	r.routes = append(r.routes, route{
		pattern: regexp.MustCompile(pattern),
		handler: handler,
		argFunc: argFunc,
	})
}

// Match finds the handler and args for a message.
func (r *Router) Match(message string) (command.Handler, string) {
	for _, rt := range r.routes {
		matches := rt.pattern.FindStringSubmatch(message)
		if matches != nil {
			args := rt.argFunc(message, matches)
			return rt.handler, args
		}
	}
	return r.fallback, message
}
