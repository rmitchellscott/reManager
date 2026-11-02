//go:build ios

#import <UIKit/UIKit.h>
#import <BackgroundTasks/BackgroundTasks.h>
#import <UserNotifications/UserNotifications.h>

// A continued processing task keeps a backgrounded app running for as long as
// it reports progress, with a Live Activity showing the title, subtitle and
// progress. It exists from iOS 26, can only be submitted from the foreground,
// and iOS may expire it. iOS alerts with a sound or haptic whenever one ends,
// so it is submitted only as the app resigns active, the last moment it still
// counts as foreground. It is dismissed when the app returns from the
// background rather than on becoming active, since locking the device can flick
// the app back to active before it leaves. The UIKit background task runs
// beside it as the fallback, which buys about 30 seconds after the app leaves
// the foreground.

static const int64_t progressUnits = 100000;
// A finished job can still be waiting on the user, and the heartbeat needs
// units left to tick, so a reported 100% fills the bar to 99%.
static const int64_t heartbeatHeadroom = 1000;

// iOS expires a task whose completed unit count stands still for about half a
// minute, and a job only reports progress between its steps, which can each
// take longer. Between reports the count ticks up by a single unit, far too
// small to move the bar, so a job that is still running is not taken for stalled.
static const NSTimeInterval heartbeatInterval = 5;
static NSTimer *heartbeatTimer = nil;

static NSString *shownTitle = @"";
static NSString *shownSubtitle = @"";
static double shownFraction = -1;

// Bumped on every end, so a task that launches after its work already ended
// completes at once instead of showing a finished job.
static NSUInteger workGeneration = 0;
static NSUInteger submissionCount = 0;
static BOOL workRunning = NO;
static BOOL continuedTaskSubmitted = NO;
static id continuedTask = nil;

static UIBackgroundTaskIdentifier graceTask;
static BOOL graceTaskActive = NO;

static void endGraceTask(void) {
    if (graceTaskActive) {
        [[UIApplication sharedApplication] endBackgroundTask:graceTask];
        graceTaskActive = NO;
    }
}

static void beginGraceTask(void) {
    if (graceTaskActive) {
        return;
    }
    graceTask = [[UIApplication sharedApplication] beginBackgroundTaskWithName:@"reManager.work"
                                                             expirationHandler:^{
        endGraceTask();
    }];
    graceTaskActive = graceTask != UIBackgroundTaskInvalid;
}

API_AVAILABLE(ios(26.0))
static void applyToContinuedTask(BGContinuedProcessingTask *task) {
    [task updateTitle:shownTitle subtitle:shownSubtitle];
    task.progress.totalUnitCount = progressUnits;
    int64_t reported = (int64_t)(shownFraction * (progressUnits - heartbeatHeadroom));
    if (shownFraction >= 0 && reported > task.progress.completedUnitCount) {
        task.progress.completedUnitCount = reported;
    }
}

static void stopHeartbeat(void) {
    [heartbeatTimer invalidate];
    heartbeatTimer = nil;
}

API_AVAILABLE(ios(26.0))
static void startHeartbeat(void) {
    stopHeartbeat();
    heartbeatTimer = [NSTimer scheduledTimerWithTimeInterval:heartbeatInterval repeats:YES block:^(NSTimer *timer) {
        BGContinuedProcessingTask *task = continuedTask;
        if (task == nil) {
            stopHeartbeat();
            return;
        }
        if (task.progress.completedUnitCount + 1 < progressUnits) {
            task.progress.completedUnitCount += 1;
        }
    }];
}

// A job asks for attention once the user has left, and iOS shows the permission
// prompt only to an app in the foreground, so it is asked as a job starts. iOS
// shows it until the user answers and never again after.
static void requestNotificationPermission(void) {
    [[UNUserNotificationCenter currentNotificationCenter]
        requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound)
                      completionHandler:^(BOOL granted, NSError *error) {}];
}

// Completing a task with failure ends its Live Activity without the alert that
// a successful completion plays.
static void dismissContinuedTask(void) {
    stopHeartbeat();
    BGTask *task = continuedTask;
    continuedTask = nil;
    continuedTaskSubmitted = NO;
    [task setTaskCompletedWithSuccess:NO];
}

API_AVAILABLE(ios(26.0))
static void submitContinuedTask(void) {
    // Each identifier can be registered only once, so every submission takes a
    // fresh one under the wildcard in BGTaskSchedulerPermittedIdentifiers.
    NSString *identifier = [NSString stringWithFormat:@"%@.work.%lu",
                            [[NSBundle mainBundle] bundleIdentifier], (unsigned long)++submissionCount];
    NSUInteger generation = workGeneration;
    BGTaskScheduler *scheduler = [BGTaskScheduler sharedScheduler];
    [scheduler registerForTaskWithIdentifier:identifier usingQueue:nil launchHandler:^(BGTask *launched) {
        BGContinuedProcessingTask *task = (BGContinuedProcessingTask *)launched;
        dispatch_async(dispatch_get_main_queue(), ^{
            if (generation != workGeneration) {
                [task setTaskCompletedWithSuccess:YES];
                return;
            }
            continuedTask = task;
            __weak BGContinuedProcessingTask *weakTask = task;
            task.expirationHandler = ^{
                dispatch_async(dispatch_get_main_queue(), ^{
                    BGContinuedProcessingTask *expired = weakTask;
                    if (expired == nil || continuedTask != expired) {
                        return;
                    }
                    NSLog(@"reManager: background work expired");
                    dismissContinuedTask();
                });
            };
            applyToContinuedTask(task);
            startHeartbeat();
        });
    }];

    BGContinuedProcessingTaskRequest *request =
        [[BGContinuedProcessingTaskRequest alloc] initWithIdentifier:identifier
                                                               title:shownTitle
                                                            subtitle:shownSubtitle];
    request.strategy = BGContinuedProcessingTaskRequestSubmissionStrategyFail;
    NSError *error = nil;
    continuedTaskSubmitted = [scheduler submitTaskRequest:request error:&error];
    if (!continuedTaskSubmitted) {
        NSLog(@"reManager: could not submit background work: %@", error);
    }
}

static void observeAppActivation(void) {
    static dispatch_once_t once;
    dispatch_once(&once, ^{
        NSNotificationCenter *center = [NSNotificationCenter defaultCenter];
        [center addObserverForName:UIApplicationWillResignActiveNotification
                            object:nil
                             queue:[NSOperationQueue mainQueue]
                        usingBlock:^(NSNotification *notification) {
            if (@available(iOS 26.0, *)) {
                if (workRunning && continuedTask == nil && !continuedTaskSubmitted) {
                    submitContinuedTask();
                }
            }
        }];
        [center addObserverForName:UIApplicationWillEnterForegroundNotification
                            object:nil
                             queue:[NSOperationQueue mainQueue]
                        usingBlock:^(NSNotification *notification) {
            if (continuedTask != nil) {
                dismissContinuedTask();
            }
        }];
    });
}

void ios_show_background_work(const char *title, const char *subtitle, double fraction) {
    NSString *titleCopy = [NSString stringWithUTF8String:title];
    NSString *subtitleCopy = [NSString stringWithUTF8String:subtitle];
    dispatch_async(dispatch_get_main_queue(), ^{
        shownTitle = titleCopy;
        shownSubtitle = subtitleCopy;
        shownFraction = fraction;
        workRunning = YES;
        observeAppActivation();

        BOOL inForeground = [UIApplication sharedApplication].applicationState != UIApplicationStateBackground;
        if (inForeground) {
            if (!graceTaskActive) {
                requestNotificationPermission();
            }
            beginGraceTask();
        }
        if (@available(iOS 26.0, *)) {
            if (continuedTask != nil) {
                applyToContinuedTask(continuedTask);
            }
        }
    });
}

static void finishBackgroundWork(BOOL succeeded) {
    workGeneration++;
    workRunning = NO;
    continuedTaskSubmitted = NO;
    stopHeartbeat();
    if (@available(iOS 26.0, *)) {
        BGContinuedProcessingTask *task = continuedTask;
        continuedTask = nil;
        if (task != nil) {
            if (succeeded) {
                task.progress.completedUnitCount = task.progress.totalUnitCount;
            }
            [task setTaskCompletedWithSuccess:succeeded];
        }
    }
    endGraceTask();
}

static void postNotification(NSString *identifier, NSString *title, NSString *subtitle, NSString *body) {
    if ([UIApplication sharedApplication].applicationState == UIApplicationStateActive) {
        return;
    }
    UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
    content.title = title;
    content.subtitle = subtitle;
    content.body = body;
    content.sound = [UNNotificationSound defaultSound];
    UNNotificationRequest *request = [UNNotificationRequest requestWithIdentifier:identifier
                                                                          content:content
                                                                          trigger:nil];
    [[UNUserNotificationCenter currentNotificationCenter] addNotificationRequest:request withCompletionHandler:nil];
}

// A failed continued processing task leaves nothing on screen, since iOS
// dismisses its Live Activity, so a user who was not watching hears of the
// failure through a notification instead.
void ios_fail_background_work(const char *title, const char *message) {
    NSString *titleCopy = [NSString stringWithUTF8String:title];
    NSString *messageCopy = [NSString stringWithUTF8String:message];
    dispatch_async(dispatch_get_main_queue(), ^{
        finishBackgroundWork(NO);
        postNotification(@"reManager.job-failed", titleCopy, @"Failed", messageCopy);
    });
}

void ios_end_background_work(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        finishBackgroundWork(YES);
    });
}

void ios_notify_needs_attention(const char *title, const char *body) {
    NSString *titleCopy = [NSString stringWithUTF8String:title];
    NSString *bodyCopy = [NSString stringWithUTF8String:body];
    dispatch_async(dispatch_get_main_queue(), ^{
        postNotification(@"reManager.needs-attention", titleCopy, @"", bodyCopy);
    });
}
