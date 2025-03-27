// Copyright Kronicler Authors
// SPDX-License-Identifier: Apache-2.0

package observability

import (
	"net/http/pprof"
	"os"

	"github.com/gin-gonic/gin"
)

// EnablePprofEnvVar controls if the Pprof support is added or not.
// It is exported so the Operator can use it.
const EnablePprofEnvVar = "KRONICLER_ENABLE_PPROF"

// SetupPprof adds the different  pprof profiles to a gin.Engine
// under the well-known endpoint `/debug/pprof/`
func SetupPprof(router *gin.Engine) {
	if os.Getenv(EnablePprofEnvVar) == "true" {
		router.GET("/debug/pprof/", gin.WrapF(pprof.Index))
		router.GET("/debug/pprof/cmdline", gin.WrapF(pprof.Cmdline))
		router.GET("/debug/pprof/profile", gin.WrapF(pprof.Profile))
		router.POST("/debug/pprof/symbol", gin.WrapF(pprof.Symbol))
		router.GET("/debug/pprof/symbol", gin.WrapF(pprof.Symbol))
		router.GET("/debug/pprof/trace", gin.WrapF(pprof.Trace))
		router.GET("/debug/pprof/allocs", gin.WrapH(pprof.Handler("allocs")))
		router.GET("/debug/pprof/block", gin.WrapH(pprof.Handler("block")))
		router.GET("/debug/pprof/goroutine", gin.WrapH(pprof.Handler("goroutine")))
		router.GET("/debug/pprof/heap", gin.WrapH(pprof.Handler("heap")))
		router.GET("/debug/pprof/mutex", gin.WrapH(pprof.Handler("mutex")))
		router.GET("/debug/pprof/threadcreate", gin.WrapH(pprof.Handler("threadcreate")))
	}
}
