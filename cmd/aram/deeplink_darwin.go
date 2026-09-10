//go:build darwin && cgo && !android && !ios

package main

/*
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>
void aramInstallURLHandler(void);
*/
import "C"

import "runtime"

func configurePlatformDeepLinks() error {
	runtime.LockOSThread()
	C.aramInstallURLHandler()
	return nil
}

//export aramHandleURL
func aramHandleURL(raw *C.char) {
	if raw == nil {
		return
	}
	enqueuePlatformDeepLink(C.GoString(raw))
}
