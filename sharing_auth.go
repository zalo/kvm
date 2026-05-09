package kvm

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// sharingAuthToken is an in-memory token issued to guests who log in with the
// sharing password. Regenerated each boot so old links die after a reboot.
// Empty until the first guest logs in (or always empty if no sharing
// password is set).
var sharingAuthToken string

// handleSharingLogin authenticates a guest against config.SharingPasswordHash
// and sets a `shareToken` cookie on success. Used to gate WebRTC signaling
// for non-admin viewers in the cloud-gaming co-op model.
func handleSharingLogin(c *gin.Context) {
	if config.SharingPasswordHash == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Sharing not enabled"})
		return
	}

	ip := c.ClientIP()
	if allowed, retryAfter := passwordRateLimiter.IsAllowed(ip); !allowed {
		c.Header("Retry-After", retryAfterHeaderValue(retryAfter))
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error":       "Too many failed attempts. Please try again later.",
			"retry_after": retryAfter,
		})
		return
	}

	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(config.SharingPasswordHash), []byte(req.Password)); err != nil {
		passwordRateLimiter.RecordFailure(ip)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid sharing password"})
		return
	}
	passwordRateLimiter.RecordSuccess(ip)

	if sharingAuthToken == "" {
		sharingAuthToken = uuid.New().String()
	}
	c.SetCookie("shareToken", sharingAuthToken, authTokenMaxAge, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"message": "Login successful"})
}

// webrtcAuthMiddleware accepts EITHER an admin authToken OR a shareToken
// minted by handleSharingLogin. Admin always wins; sharing is allowed only
// when SharingPasswordHash is set (otherwise this collapses to the regular
// admin-only path so we don't accidentally widen access).
func webrtcAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if config.LocalAuthMode == "noPassword" {
			c.Next()
			return
		}

		// Admin path: same check as protectedMiddleware.
		if t, err := c.Cookie("authToken"); err == nil && t != "" && t == config.LocalAuthToken {
			c.Next()
			return
		}

		// Guest path: only valid when a sharing password is configured AND the
		// presented shareToken matches the in-memory token from this boot.
		if config.SharingPasswordHash != "" && sharingAuthToken != "" {
			if t, err := c.Cookie("shareToken"); err == nil && t == sharingAuthToken {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		c.Abort()
	}
}

func retryAfterHeaderValue(seconds int) string {
	// avoid pulling fmt for one call site
	if seconds <= 0 {
		return "1"
	}
	const digits = "0123456789"
	if seconds < 10 {
		return string(digits[seconds])
	}
	// fall back to fmt.Sprintf-style for >=10
	out := []byte{}
	for seconds > 0 {
		out = append([]byte{digits[seconds%10]}, out...)
		seconds /= 10
	}
	return string(out)
}

// rpcSetSharingPassword stores a bcrypt hash of the given password (or clears
// it if password is empty). Empty password disables sharing entirely.
func rpcSetSharingPassword(password string) error {
	if password == "" {
		config.SharingPasswordHash = ""
		sharingAuthToken = ""
	} else {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		config.SharingPasswordHash = string(hash)
		// Rotate the in-memory token so any old guest sessions are invalidated.
		sharingAuthToken = uuid.New().String()
	}
	return SaveConfig()
}

// rpcGetSharingPasswordSet returns whether a sharing password is configured.
// We never expose the hash itself.
func rpcGetSharingPasswordSet() (bool, error) {
	return config.SharingPasswordHash != "", nil
}

// rpcSetMultiPlayerGamepad toggles per-session HID gamepad slot mapping.
func rpcSetMultiPlayerGamepad(enabled bool) error {
	config.MultiPlayerGamepad = enabled
	return SaveConfig()
}

// rpcGetMultiPlayerGamepad returns the current value.
func rpcGetMultiPlayerGamepad() (bool, error) {
	return config.MultiPlayerGamepad, nil
}
