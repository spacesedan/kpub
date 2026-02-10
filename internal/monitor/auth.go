package monitor

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

// terminalAuth implements auth.UserAuthenticator for interactive terminal login.
type terminalAuth struct{}

var _ auth.UserAuthenticator = terminalAuth{}

func (a terminalAuth) Phone(_ context.Context) (string, error) {
	fmt.Print("Enter your phone number (e.g. +1234567890): ")
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return "", fmt.Errorf("failed to read phone number")
	}
	return strings.TrimSpace(scanner.Text()), nil
}

func (a terminalAuth) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	fmt.Print("Enter Telegram verification code: ")
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return "", fmt.Errorf("failed to read verification code")
	}
	return strings.TrimSpace(scanner.Text()), nil
}

func (a terminalAuth) Password(_ context.Context) (string, error) {
	fmt.Print("Enter 2FA password: ")
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return "", fmt.Errorf("failed to read password")
	}
	return strings.TrimSpace(scanner.Text()), nil
}

func (a terminalAuth) AcceptTermsOfService(_ context.Context, _ tg.HelpTermsOfService) error {
	return nil
}

func (a terminalAuth) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, fmt.Errorf("sign-up not supported")
}
