package handlers

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"triageguard/apps/api/internal/db"
)

type requestReportLookup struct {
	channelByID map[string]string
	ownerByID   map[string]string
}

func (a *App) buildRequestReportLookup(ctx context.Context, workspaceID uuid.UUID, rows []db.RequestReportRow) requestReportLookup {
	lookup := requestReportLookup{
		channelByID: map[string]string{},
		ownerByID:   map[string]string{},
	}
	if len(rows) == 0 {
		return lookup
	}

	for _, row := range rows {
		channelID := strings.TrimSpace(row.ChannelID)
		channelName := ""
		if row.ChannelName != nil {
			channelName = strings.TrimSpace(*row.ChannelName)
		}
		if channelID != "" && channelName != "" {
			lookup.channelByID[channelID] = channelName
		}
	}

	channels, err := a.store.ListChannels(ctx, workspaceID)
	if err == nil {
		for _, channel := range channels {
			channelID := strings.TrimSpace(channel.ChannelID)
			if channelID == "" {
				continue
			}
			channelName := strings.TrimSpace(channel.ChannelName)
			if channelName == "" {
				continue
			}
			lookup.channelByID[channelID] = channelName
		}
	}

	ownerIDs := map[string]struct{}{}
	for _, row := range rows {
		if row.OwnerSlackID == nil {
			continue
		}
		ownerID := strings.TrimSpace(*row.OwnerSlackID)
		if ownerID == "" {
			continue
		}
		ownerIDs[ownerID] = struct{}{}
	}

	if len(ownerIDs) == 0 {
		return lookup
	}

	install, err := a.store.GetSlackInstallationByWorkspace(ctx, workspaceID)
	if err != nil {
		return lookup
	}

	for ownerID := range ownerIDs {
		displayName, err := a.slackClient.GetUserDisplayName(ctx, install.BotToken, ownerID)
		if err != nil {
			a.logger.Printf("reports: owner display lookup failed workspace=%s owner=%s: %v", workspaceID, ownerID, err)
			continue
		}
		displayName = strings.TrimSpace(displayName)
		if displayName == "" {
			continue
		}
		lookup.ownerByID[ownerID] = displayName
	}

	return lookup
}

func (l requestReportLookup) channelDisplay(channelID string) string {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return "-"
	}
	if channelName := strings.TrimSpace(l.channelByID[channelID]); channelName != "" {
		return "#" + channelName
	}
	return channelID
}

func (l requestReportLookup) ownerDisplay(ownerSlackID *string) string {
	if ownerSlackID == nil {
		return "-"
	}
	ownerID := strings.TrimSpace(*ownerSlackID)
	if ownerID == "" {
		return "-"
	}
	if ownerName := strings.TrimSpace(l.ownerByID[ownerID]); ownerName != "" {
		return ownerName
	}
	return ownerID
}

func linearIssueLabel(issueURL, issueID *string) string {
	if issueURL != nil {
		trimmedURL := strings.TrimSpace(*issueURL)
		if trimmedURL != "" {
			if marker := "/issue/"; strings.Contains(trimmedURL, marker) {
				parts := strings.SplitN(trimmedURL, marker, 2)
				if len(parts) == 2 {
					tail := strings.SplitN(parts[1], "/", 2)[0]
					if strings.TrimSpace(tail) != "" {
						return strings.TrimSpace(tail)
					}
				}
			}
			return trimmedURL
		}
	}
	if issueID != nil && strings.TrimSpace(*issueID) != "" {
		return strings.TrimSpace(*issueID)
	}
	return "-"
}
