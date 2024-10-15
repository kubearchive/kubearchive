// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package pagination

import (
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kubearchive/kubearchive/cmd/api/abort"
)

const (
	limitKey        = "limit"
	continueKey     = "continue"
	continueDateKey = "continueDate"
	continueUUIDKey = "continueUUID"
	defaultLimit    = "100"
	maxAllowedLimit = 1000
)

// GetValuesFromContext is a helper function for routes to retrieve the
// information needed. This is kept here so its close to the function
// that sets these values in the context (Middleware)
func GetValuesFromContext(context *gin.Context) (string, string, string) {
	return context.GetString(limitKey), context.GetString(continueUUIDKey), context.GetString(continueDateKey)
}

func CreateToken(uuid, date string) string {
	// The date is returned as a quoted string, so remove the quotes
	date = strings.TrimPrefix(date, "\"")
	date = strings.TrimSuffix(date, "\"")
	tokenString := fmt.Sprintf("%s %s", uuid, date)
	return base64.StdEncoding.EncodeToString([]byte(tokenString))
}

// Middleware validates the `limit` and `continue` query parameters
// and populates `limit` and `continueValue` in the context with their
// respective values so they are retrieved by the endpoints that need it
func Middleware() gin.HandlerFunc {
	return func(context *gin.Context) {
		limitString := context.Query(limitKey)
		continueToken := context.Query(continueKey)

		var limit string
		if limitString == "" {
			// If not specified, we set a limit so users don't retrieve
			// large collections by mistake. Those collections could
			// make us crash if they are too large
			limit = defaultLimit
		} else {
			var err error
			limitInteger, err := strconv.Atoi(limitString)
			if err != nil {
				abort.Abort(context, fmt.Sprintf("limit '%s' could not be converted to integer", limitString), http.StatusBadRequest)
				return
			}

			if limitInteger > maxAllowedLimit {
				abort.Abort(context, fmt.Sprintf("limit '%s' exceeds the maximum allowed '%d'", limitString, maxAllowedLimit), http.StatusBadRequest)
				return
			}

			// We reserialize to avoid SQL injection. There is the possibility the
			// value is a valid integer, but in SQL does something else.
			limit = strconv.Itoa(limitInteger)
		}

		var continueDate string
		var continueUUID string
		if continueToken != "" {
			continueBytes, err := base64.StdEncoding.DecodeString(continueToken)
			if err != nil {
				abort.Abort(context, "could not decode the continuation token", http.StatusBadRequest)
				return
			}

			continueParts := strings.Split(string(continueBytes), " ")
			if len(continueParts) != 2 {
				abort.Abort(context, "expected two elements on the continue token, received a different amount", http.StatusBadRequest)
				return
			}

			continueUUID = continueParts[0]
			err = uuid.Validate(continueUUID)
			if err != nil {
				abort.Abort(context, "first element of the continue token is not a valid UUID", http.StatusBadRequest)
				return
			}

			continueDate = continueParts[1]
			continueTimestamp, err := time.Parse(time.RFC3339, continueDate)
			if err != nil {
				log.Printf("Error: %s", err)
				abort.Abort(context, fmt.Sprintf("continuation token decoded value '%s' is not valid. It should match '%s'", continueDate, time.RFC3339), http.StatusBadRequest)
				return
			}

			// We reserialize to avoid SQL injection. There is the possibility the
			// value is a valid date, but in SQL does something else.
			continueDate = continueTimestamp.Format(time.RFC3339)
		}

		// Pass the values using the context, these should be retrieved
		// using `GetValuesFromContext`
		context.Set(limitKey, limit)
		context.Set(continueDateKey, continueDate)
		context.Set(continueUUIDKey, continueUUID)
	}
}
