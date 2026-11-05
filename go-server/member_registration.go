package main

import (
	"context"
	"net/url"
)

// The guest-readable plans endpoint identifies the CMS generation before the
// first registration POST. A failed probe cannot create an account.
func (a *app) registerCMSMember(ctx context.Context, s *memberSession, params url.Values) (bool, error) {
	if _, err := a.memberCall(ctx, s, "api.php/user/ajax_upgrade_data", "GET", nil); err != nil {
		return false, err
	}
	if _, err := a.memberCall(ctx, s, "api.php/user/register", "POST", params); err != nil {
		return false, err
	}
	s.Mu.Lock()
	defer s.Mu.Unlock()
	// Native legacy reg already logs in, including its own registration checks.
	// Repeating login would lose that session when the CMS requires a captcha.
	return s.LegacyAPI, nil
}
