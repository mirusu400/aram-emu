//go:build darwin && cgo

#import <Cocoa/Cocoa.h>

extern void aramHandleURL(char *raw);

@interface ARAMURLHandler : NSObject
- (void)handleGetURLEvent:(NSAppleEventDescriptor *)event
           withReplyEvent:(NSAppleEventDescriptor *)replyEvent;
@end

@implementation ARAMURLHandler
- (void)handleGetURLEvent:(NSAppleEventDescriptor *)event
           withReplyEvent:(NSAppleEventDescriptor *)replyEvent {
    (void)replyEvent;
    NSString *value = [[event paramDescriptorForKeyword:keyDirectObject] stringValue];
    if (value == nil) {
        return;
    }
    aramHandleURL((char *)[value UTF8String]);
}
@end

static ARAMURLHandler *aramURLHandler;

void aramInstallURLHandler(void) {
    [NSApplication sharedApplication];
    aramURLHandler = [[ARAMURLHandler alloc] init];
    [[NSAppleEventManager sharedAppleEventManager]
        setEventHandler:aramURLHandler
             andSelector:@selector(handleGetURLEvent:withReplyEvent:)
           forEventClass:kInternetEventClass
              andEventID:kAEGetURL];
}
