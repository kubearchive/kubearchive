// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0

package pagination

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kubearchive/kubearchive/cmd/api/abort"
)

const (
	limitKey        = "limit"
	continueKey     = "continue"
	continueDateKey = "continueDate"
	continueIdKey   = "continueId"
	defaultLimit    = "100"
	maxAllowedLimit = 1000
)

// GetValuesFromContext is a helper function for routes to retrieve the
// information needed. This is kept here, so it's close to the function
// that sets these values in the context (Middleware)
func GetValuesFromContext(context *gin.Context) (string, string, string) {
	return context.GetString(limitKey), context.GetString(continueIdKey), context.GetString(continueDateKey)
}

func CreateToken(uuid int64, date string) string {
	// The date is returned as a quoted string, so remove the quotes
	date = strings.TrimPrefix(date, "\"")
	date = strings.TrimSuffix(date, "\"")
	if date == "" && uuid == 0 {
		return ""
	}
	tokenString := fmt.Sprintf("%d %s", uuid, date)
	return base64.StdEncoding.EncodeToString([]byte(tokenString))
}

// Middleware validates the `limit` and `continue` query parameters
// and populates `limit` and `continueValue` in the context with their
// respective values, so they are retrieved by the endpoints that need it
func Middleware() gin.HandlerFunc {
	return func(context *gin.Context) {
		// We always use a default limit because we don't want to return
		// large collections if users don't remember to specify a limit
		limitString := context.DefaultQuery(limitKey, defaultLimit)
		continueToken := context.Query(continueKey)

		var limit string
		limitInteger, err := strconv.Atoi(limitString)
		if err != nil {
			abort.Abort(context, fmt.Errorf("limit '%s' could not be converted to integer", limitString), http.StatusBadRequest)
			return
		}
		if limitInteger > maxAllowedLimit {
			abort.Abort(context, fmt.Errorf("limit '%s' exceeds the maximum allowed '%d'", limitString, maxAllowedLimit), http.StatusBadRequest)
			return
		}
		// We reserialize to avoid SQL injection. There is the possibility the
		// value is a valid integer, but in SQL does something else.
		limit = strconv.Itoa(limitInteger)

		var continueDate string
		var continueId string
		if continueToken != "" {
			continueBytes, err := base64.StdEncoding.DecodeString(continueToken)
			if err != nil {
				abort.Abort(context, errors.New("could not decode the continuation token"), http.StatusBadRequest)
				return
			}

			continueParts := strings.Split(string(continueBytes), " ")
			if len(continueParts) != 2 {
				abort.Abort(context, errors.New("expected two elements on the continue token, received a different amount"), http.StatusBadRequest)
				return
			}

			continueId = continueParts[0]
			// Because the id is an int64 we need to use `ParseInt`
			_, err = strconv.ParseInt(continueId, 10, 64)
			if err != nil {
				abort.Abort(context, errors.New("first element of the continue token is not a valid int64"), http.StatusBadRequest)
				return
			}

			continueDate = continueParts[1]
			continueTimestamp, err := time.Parse(time.RFC3339, continueDate)
			if err != nil {
				log.Printf("Error: %s", err)
				abort.Abort(context, fmt.Errorf("second element of the continue token '%s' does not match '%s'",
					continueDate, time.RFC3339), http.StatusBadRequest)
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
		context.Set(continueIdKey, continueId)
	}
}
