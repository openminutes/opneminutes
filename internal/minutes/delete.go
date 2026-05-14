package minutes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"go.uber.org/zap"
)

const defaultDeleteSpaceName = 1

// DeleteMinute moves a minute to trash, and optionally permanently deletes it.
func (c *Client) DeleteMinute(ctx context.Context, objectToken string, options DeleteOptions) error {
	objectToken = strings.TrimSpace(objectToken)
	if objectToken == "" {
		return errors.New("object token is required")
	}

	spaceName, err := deleteSpaceName(options.SpaceName)
	if err != nil {
		return err
	}
	language := defaultString(options.Language, defaultLanguage)

	c.logger.Debug("delete minute started",
		zap.String("object_token", objectToken),
		zap.String("language", language),
		zap.Int("space_name", spaceName),
		zap.Bool("destroy", options.Destroy),
	)

	if err := c.removeMinuteFromSpace(ctx, objectToken, language, spaceName); err != nil {
		c.logger.Debug("delete minute remove failed",
			zap.String("object_token", objectToken),
			zap.Error(err),
		)
		return err
	}
	if err := c.deleteMinuteFromTrash(ctx, objectToken, language, false); err != nil {
		c.logger.Debug("delete minute trash move failed",
			zap.String("object_token", objectToken),
			zap.Error(err),
		)
		return err
	}
	if options.Destroy {
		if err := c.deleteMinuteFromTrash(ctx, objectToken, language, true); err != nil {
			c.logger.Debug("delete minute destroy failed",
				zap.String("object_token", objectToken),
				zap.Error(err),
			)
			return err
		}
	}

	c.logger.Debug("delete minute completed",
		zap.String("object_token", objectToken),
		zap.Bool("destroy", options.Destroy),
	)
	return nil
}

func (c *Client) removeMinuteFromSpace(ctx context.Context, objectToken, language string, spaceName int) error {
	form := url.Values{}
	form.Set("language", language)
	form.Set("object_tokens", objectToken)
	form.Set("space_name", strconv.Itoa(spaceName))

	return c.postMinutesForm(ctx, "/minutes/api/space/remove", form)
}

func (c *Client) deleteMinuteFromTrash(ctx context.Context, objectToken, language string, destroyed bool) error {
	form := url.Values{}
	form.Set("is_destroyed", strconv.FormatBool(destroyed))
	form.Set("language", language)
	form.Set("object_tokens", objectToken)

	return c.postMinutesForm(ctx, "/minutes/api/space/delete", form)
}

func (c *Client) postMinutesForm(ctx context.Context, path string, form url.Values) error {
	req, err := c.newAPIRequest(ctx, http.MethodPost, path, nil, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/x-www-form-urlencoded")

	return c.doJSON(req, nil)
}

func deleteSpaceName(spaceName int) (int, error) {
	if spaceName < 0 {
		return 0, fmt.Errorf("space name must be greater than 0")
	}
	if spaceName == 0 {
		return defaultDeleteSpaceName, nil
	}

	return spaceName, nil
}
