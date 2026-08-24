#import "voice_darwin.h"

#import <AVFoundation/AVFoundation.h>
#import <Foundation/Foundation.h>
#import <Speech/Speech.h>
#import <objc/runtime.h>
#import <os/lock.h>

extern void goVoicePartial(const char *text);
extern void goVoiceFinal(const char *text);
extern void goVoiceError(const char *text);

static os_unfair_lock gLock = OS_UNFAIR_LOCK_INIT;
static SFSpeechRecognizer *gRecognizer;
static SFSpeechAudioBufferRecognitionRequest *gRequest;
static SFSpeechRecognitionTask *gTask;
static AVAudioEngine *gEngine;
static int gStartGen;

static void emitC(void (*fn)(const char *), NSString *text) {
	if (!fn) {
		return;
	}
	NSString *copy = [(text ?: @"") copy];
	fn(copy.UTF8String);
}

static void emitErr(const char *msg) {
	emitC(goVoiceError, msg ? [NSString stringWithUTF8String:msg] : @"denied");
}

static BOOL isCancelError(NSError *error) {
	if (!error) {
		return NO;
	}
	if (error.code == 216 || error.code == 301) {
		return YES;
	}
	return NO;
}

static BOOL formatOK(AVAudioFormat *fmt) {
	return fmt && fmt.sampleRate >= 1.0 && fmt.channelCount >= 1;
}

static AVAudioFormat *tapFormat(AVAudioInputNode *input) {
	AVAudioFormat *fmt = nil;
	@try {
		// installTap requires the node's *output* format (or nil). Using the
		// hardware input format when it differs is a common first-buffer crash.
		fmt = [input outputFormatForBus:0];
		if (!formatOK(fmt)) {
			fmt = [input inputFormatForBus:0];
		}
	} @catch (NSException *ex) {
		return nil;
	}
	return formatOK(fmt) ? fmt : nil;
}

static void svpGrantMediaCapture(
	id self,
	SEL sel,
	id webView,
	id origin,
	id frame,
	NSInteger type,
	void (^decisionHandler)(NSInteger)) {
	(void)self;
	(void)sel;
	(void)webView;
	(void)origin;
	(void)frame;
	(void)type;
	if (decisionHandler) {
		decisionHandler(1); // WKPermissionDecisionGrant
	}
}

void SVPInstallWebViewMicGrant(void) {
	if (@available(macOS 12.0, *)) {
		Class cls = NSClassFromString(@"WailsContext");
		if (!cls) {
			return;
		}
		SEL sel = @selector(webView:requestMediaCapturePermissionForOrigin:initiatedByFrame:type:decisionHandler:);
		Method existing = class_getInstanceMethod(cls, sel);
		if (existing) {
			method_setImplementation(existing, (IMP)svpGrantMediaCapture);
			return;
		}
		class_addMethod(cls, sel, (IMP)svpGrantMediaCapture, "v@:@@@q@?");
	}
}

static void stopEngine(void) {
	os_unfair_lock_lock(&gLock);
	gStartGen++;
	SFSpeechRecognitionTask *task = gTask;
	gTask = nil;
	SFSpeechAudioBufferRecognitionRequest *req = gRequest;
	gRequest = nil;
	AVAudioEngine *engine = gEngine;
	gEngine = nil;
	gRecognizer = nil;
	os_unfair_lock_unlock(&gLock);

	if (task) {
		@try {
			[task cancel];
		} @catch (NSException *ex) {
		}
	}
	if (req) {
		@try {
			[req endAudio];
		} @catch (NSException *ex) {
		}
	}
	if (engine) {
		@try {
			[engine.inputNode removeTapOnBus:0];
		} @catch (NSException *ex) {
		}
		@try {
			if (engine.isRunning) {
				[engine stop];
			}
			[engine reset];
		} @catch (NSException *ex) {
		}
	}
}

void SVPVoiceStop(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		stopEngine();
	});
}

static void startOnMainAttempt(NSString *localeID, int attempt, int gen);

static void startOnMain(NSString *localeID) {
	stopEngine();
	os_unfair_lock_lock(&gLock);
	int gen = gStartGen;
	os_unfair_lock_unlock(&gLock);
	startOnMainAttempt(localeID, 0, gen);
}

static void startOnMainAttempt(NSString *localeID, int attempt, int gen) {
	os_unfair_lock_lock(&gLock);
	int current = gStartGen;
	os_unfair_lock_unlock(&gLock);
	if (gen != current) {
		return;
	}

	@try {
		NSLocale *locale = [NSLocale localeWithLocaleIdentifier:localeID];
		SFSpeechRecognizer *recognizer = [[SFSpeechRecognizer alloc] initWithLocale:locale];
		if (!recognizer) {
			recognizer = [[SFSpeechRecognizer alloc] init];
		}
		if (!recognizer || !recognizer.isAvailable) {
			emitErr("unavailable");
			return;
		}

		AVAudioEngine *engine = [[AVAudioEngine alloc] init];
		AVAudioInputNode *input = engine.inputNode;
		[engine prepare];

		AVAudioFormat *fmt = tapFormat(input);
		if (!fmt) {
			if (attempt < 8) {
				dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 120 * NSEC_PER_MSEC), dispatch_get_main_queue(), ^{
					startOnMainAttempt(localeID, attempt + 1, gen);
				});
				return;
			}
			emitErr("unavailable");
			return;
		}

		SFSpeechAudioBufferRecognitionRequest *request = [[SFSpeechAudioBufferRecognitionRequest alloc] init];
		request.shouldReportPartialResults = YES;
		request.taskHint = SFSpeechRecognitionTaskHintDictation;
		if (@available(macOS 13.0, *)) {
			request.addsPunctuation = YES;
		}

		SFSpeechRecognitionTask *task = [recognizer recognitionTaskWithRequest:request
			resultHandler:^(SFSpeechRecognitionResult *result, NSError *error) {
				@try {
					if (error) {
						if (isCancelError(error)) {
							return;
						}
						emitC(goVoiceError, error.localizedDescription ?: @"unavailable");
						return;
					}
					NSString *text = result.bestTranscription.formattedString ?: @"";
					if (result.isFinal) {
						emitC(goVoiceFinal, text);
					} else {
						emitC(goVoicePartial, text);
					}
				} @catch (NSException *ex) {
				}
			}];

		os_unfair_lock_lock(&gLock);
		if (gen != gStartGen) {
			os_unfair_lock_unlock(&gLock);
			[task cancel];
			return;
		}
		gRecognizer = recognizer;
		gEngine = engine;
		gRequest = request;
		gTask = task;
		os_unfair_lock_unlock(&gLock);

		[input installTapOnBus:0
			bufferSize:1024
			format:fmt
			block:^(AVAudioPCMBuffer *buffer, AVAudioTime *when) {
				(void)when;
				@autoreleasepool {
					if (!buffer || buffer.frameLength == 0) {
						return;
					}
					os_unfair_lock_lock(&gLock);
					SFSpeechAudioBufferRecognitionRequest *req = gRequest;
					os_unfair_lock_unlock(&gLock);
					if (!req) {
						return;
					}
					@try {
						[req appendAudioPCMBuffer:buffer];
					} @catch (NSException *ex) {
					}
				}
			}];

		NSError *err = nil;
		if (![engine startAndReturnError:&err]) {
			stopEngine();
			emitC(goVoiceError, err.localizedDescription ?: @"denied");
		}
	} @catch (NSException *ex) {
		stopEngine();
		emitC(goVoiceError, ex.reason ?: @"unavailable");
	}
}

static void requestMicrophone(void (^done)(BOOL granted)) {
	[AVCaptureDevice requestAccessForMediaType:AVMediaTypeAudio completionHandler:done];
}

void SVPVoiceStart(const char *lang) {
	NSString *localeID = @"en-US";
	if (lang && lang[0] != '\0') {
		localeID = [NSString stringWithUTF8String:lang];
	}
	[SFSpeechRecognizer requestAuthorization:^(SFSpeechRecognizerAuthorizationStatus status) {
		if (status != SFSpeechRecognizerAuthorizationStatusAuthorized) {
			emitErr("denied");
			return;
		}
		requestMicrophone(^(BOOL granted) {
			if (!granted) {
				emitErr("denied");
				return;
			}
			dispatch_async(dispatch_get_main_queue(), ^{
				startOnMain(localeID);
			});
		});
	}];
}
