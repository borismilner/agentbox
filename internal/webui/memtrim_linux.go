//go:build linux

package webui

/*
#include <malloc.h>
*/
import "C"

import (
	"os"
	"runtime/debug"
)

// releaseMemory returns the Go heap's free pages and then glibc's: malloc_trim
// walks every arena and madvises the free chunks inside it, not only the top.
func releaseMemory() {
	debug.FreeOSMemory()
	C.malloc_trim(0)
}

// GTK 4.14 learns which dmabuf formats it can import by initialising Vulkan,
// and the loader then dlopens EVERY installed driver - intel, radeon, nouveau,
// lavapipe, asahi, virtio, gfxstream - measured at 14 MB PSS in the daemon the
// first time WebKit hands a frame over. The renderer is GL (NglRenderer)
// either way, and EGL answers the dmabuf question on its own, so nothing on
// screen changes. GDK_DISABLE is the 4.16+ spelling. A value the user set is
// left alone. This runs at package init, before anything can initialise GTK.
func init() {
	if os.Getenv("GDK_DEBUG") == "" {
		_ = os.Setenv("GDK_DEBUG", "vulkan-disable")
	}
	if os.Getenv("GDK_DISABLE") == "" {
		_ = os.Setenv("GDK_DISABLE", "vulkan")
	}
}
