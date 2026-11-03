package realtime

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

const ChannelSystemNotifications = "system.notifications"

// ChannelAuthorizer decides whether a principal may subscribe to a channel.
type ChannelAuthorizer interface {
	CanSubscribeChannel(ctx context.Context, userUUID uuid.UUID, isSuperAdmin bool, channel string) (bool, error)
}

// ParseUserChannel returns the user UUID from "user:{uuid}".
func ParseUserChannel(channel string) (uuid.UUID, bool) {
	raw, ok := stripPrefix(channel, "user:")
	if !ok {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// OrganizationChannelPrefix is the per-organization channel namespace.
const OrganizationChannelPrefix = "org:"

// OrganizationChannel returns "org:{uuid}".
func OrganizationChannel(orgUUID uuid.UUID) string {
	return OrganizationChannelPrefix + orgUUID.String()
}

// ParseOrganizationChannel returns the organization UUID from "org:{uuid}".
func ParseOrganizationChannel(channel string) (uuid.UUID, bool) {
	raw, ok := stripPrefix(channel, OrganizationChannelPrefix)
	if !ok {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// ParseConversationChannel returns the conversation UUID from "conversation:{uuid}".
func ParseConversationChannel(channel string) (uuid.UUID, bool) {
	raw, ok := stripPrefix(channel, "conversation:")
	if !ok {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

func stripPrefix(channel, prefix string) (string, bool) {
	channel = strings.TrimSpace(channel)
	if !strings.HasPrefix(channel, prefix) {
		return "", false
	}
	raw := strings.TrimPrefix(channel, prefix)
	if raw == "" {
		return "", false
	}
	return raw, true
}

// QRLoginChannelPrefix is the Centrifugo namespace for QR sign-in sessions.
// One channel per QR session / browser tab; only the backend publishes.
const QRLoginChannelPrefix = "qrlogin:"

// QRLoginChannel returns the channel for an opaque QR channel id.
func QRLoginChannel(channelID string) string {
	return QRLoginChannelPrefix + channelID
}
