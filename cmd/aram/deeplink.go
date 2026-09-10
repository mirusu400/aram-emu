//go:build (windows || linux || darwin) && !android && !ios

package main

var deepLinkEvents = make(chan string, 8)

func platformDeepLinkEvents() <-chan string {
	return deepLinkEvents
}

func enqueuePlatformDeepLink(link string) {
	select {
	case deepLinkEvents <- link:
	default:
	}
}
