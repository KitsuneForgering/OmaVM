#ifndef CEST_H_INCLUDED
#define CEST_H_INCLUDED

#define _POSIX_C_SOURCE 200809L
#define _DEFAULT_SOURCE

#include <stdio.h>
#include <string.h>
#include <stdbool.h>
#include <stdlib.h>
#include <time.h>
#ifndef _WIN32
#  include <unistd.h>
#  include <sys/wait.h>
#endif
#ifdef CEST_ENABLE_SIGNAL_HANDLER
#  include <signal.h>
#endif

#ifdef __OBJC__
#import <objc/objc.h>
#endif

#ifdef __cplusplus
#include <string>
#include <regex>
#include <initializer_list>
#endif

// ============================================================================
// Warning Suppression (enabled by default, can be disabled)
// ============================================================================
#ifndef CEST_SUPPRESS_WARNINGS
#  define CEST_SUPPRESS_WARNINGS 1
#endif

#if CEST_SUPPRESS_WARNINGS
#  ifdef __GNUC__
#    pragma GCC diagnostic push
#    pragma GCC diagnostic ignored "-Wpedantic"
#    pragma GCC diagnostic ignored "-Wstrict-prototypes"
#    pragma GCC diagnostic ignored "-Wunused-function"
#    pragma GCC diagnostic ignored "-Wunused-parameter"
#    pragma GCC diagnostic ignored "-Wimplicit-fallthrough"
#    pragma GCC diagnostic ignored "-Wcast-align"
#  endif
#  ifdef __clang__
#    pragma clang diagnostic push
#    pragma clang diagnostic ignored "-Wpedantic"
#    pragma clang diagnostic ignored "-Wstrict-prototypes"
#    pragma clang diagnostic ignored "-Wunused-function"
#    pragma clang diagnostic ignored "-Wunused-parameter"
#    pragma clang diagnostic ignored "-Wimplicit-fallthrough"
#    pragma clang diagnostic ignored "-Wcast-align"
#    pragma clang diagnostic ignored "-Wcovered-switch-default"
#  endif
#  ifdef _MSC_VER
#    pragma warning(push)
#    pragma warning(disable: 4100) // unreferenced formal parameter
#    pragma warning(disable: 4102) // unreferenced label
#    pragma warning(disable: 4189) // local variable is initialized
#    pragma warning(disable: 4201) // nameless struct/union
#    pragma warning(disable: 4214) // bitfield types other than int
#    pragma warning(disable: 4505) // unreferenced local function
#  endif
#endif

#define _CEST_C11 (__STDC_VERSION__ >= 201112L)

// ============================================================================
// CONFIGURATION MACROS (define before including cest.h)
// ============================================================================
// #define CEST_NO_COLORS             // Disable colored output
// #define CEST_THREAD_SAFE           // Enable thread safety (requires pthreads)
// #define CEST_NO_CLI                // Disable CLI argument parsing
// #define CEST_NO_HOOKS              // Disable beforeEach/afterEach hooks
// #define CEST_ENABLE_COVERAGE       // Enable gcov coverage integration
// #define CEST_ENABLE_LEAK_DETECTION // Enable memory leak detection (disabled if sanitizer active)
// #define CEST_ENABLE_SIGNAL_HANDLER // Enable crash diagnostics (SIGSEGV, SIGABRT, etc.)
// #define CEST_PREFIX                // Use cest_ prefix on all public macros

// NOTE (C++): cest.h defines short macros like `test` and `it`. Include any
// standard library headers (<iostream>, <bitset>, etc.) BEFORE including
// cest.h, or those macros can collide with unrelated identifiers such as
// std::bitset::test(). If that's not possible, define CEST_PREFIX to use
// cest_test/cest_it instead.

// ============================================================================
// Automatic Sanitizer Detection (before any configuration)
// ============================================================================
#ifndef CEST_SANITIZER_FLAGS
#  ifdef __has_feature
#    if __has_feature(address_sanitizer)
#      define CEST_ASAN_ACTIVE 1
#    endif
#    if __has_feature(thread_sanitizer)
#      define CEST_TSAN_ACTIVE 1
#    endif
#    if __has_feature(memory_sanitizer)
#      define CEST_MSAN_ACTIVE 1
#    endif
#    if __has_feature(undefined_sanitizer)
#      define CEST_UBSAN_ACTIVE 1
#    endif
#  endif
#  if defined(__SANITIZE_ADDRESS__) && !defined(CEST_ASAN_ACTIVE)
#    define CEST_ASAN_ACTIVE 1
#  endif
#  if defined(__SANITIZE_THREAD__) && !defined(CEST_TSAN_ACTIVE)
#    define CEST_TSAN_ACTIVE 1
#  endif
#  if defined(__SANITIZE_MEMORY__) && !defined(CEST_MSAN_ACTIVE)
#    define CEST_MSAN_ACTIVE 1
#  endif
#  if defined(__SANITIZE_UNDEFINED__) && !defined(CEST_UBSAN_ACTIVE)
#    define CEST_UBSAN_ACTIVE 1
#  endif

// If any sanitizer is active, disable internal leak detection to avoid conflicts
#  if defined(CEST_ASAN_ACTIVE) || defined(CEST_TSAN_ACTIVE) || defined(CEST_MSAN_ACTIVE) || defined(CEST_UBSAN_ACTIVE)
#    ifdef CEST_ENABLE_LEAK_DETECTION
#      undef CEST_ENABLE_LEAK_DETECTION
#      pragma message("Cest: Leak detection disabled because a sanitizer is active.")
#    endif
#  endif
#endif

// ============================================================================
// Sanitizer Suppression Macros (use before functions that intentionally trigger UB)
// ============================================================================
#if defined(__GNUC__) || defined(__clang__)
#  define CEST_NO_SANITIZE_ADDRESS   __attribute__((no_sanitize("address")))
#  define CEST_NO_SANITIZE_THREAD    __attribute__((no_sanitize("thread")))
#  define CEST_NO_SANITIZE_MEMORY    __attribute__((no_sanitize("memory")))
#  define CEST_NO_SANITIZE_UNDEFINED __attribute__((no_sanitize("undefined")))
#  define CEST_NO_SANITIZE_ALL       __attribute__((no_sanitize("address", "thread", "memory", "undefined")))
#elif defined(_MSC_VER)
#  define CEST_NO_SANITIZE_ADDRESS   __declspec(no_sanitize_address)
#  define CEST_NO_SANITIZE_THREAD    /* not supported */
#  define CEST_NO_SANITIZE_MEMORY    /* not supported */
#  define CEST_NO_SANITIZE_UNDEFINED /* not supported */
#  define CEST_NO_SANITIZE_ALL       /* not supported */
#else
#  define CEST_NO_SANITIZE_ADDRESS
#  define CEST_NO_SANITIZE_THREAD
#  define CEST_NO_SANITIZE_MEMORY
#  define CEST_NO_SANITIZE_UNDEFINED
#  define CEST_NO_SANITIZE_ALL
#endif

// ============================================================================
// Portability - MSVC and Cross-Platform Support
// ============================================================================
#ifndef CEST_WEAK
#  if defined(_MSC_VER)
#    if defined(__cplusplus)
#      define CEST_WEAK static
#    else
#      define CEST_WEAK __declspec(selectany) extern
#    endif
#  else
#    define CEST_WEAK __attribute__((weak))
#  endif
#endif

#ifndef CEST_UNUSED
#  if defined(_MSC_VER)
#    define CEST_UNUSED
#  elif defined(__GNUC__) || defined(__clang__)
#    define CEST_UNUSED __attribute__((unused))
#  else
#    define CEST_UNUSED
#  endif
#endif

// Thread safety support
//
// Serializes access to the shared stats counters (_cest_global_stats) and
// makes the per-assertion context (_cest_ctx) thread-local, so worker
// threads spawned from inside a test body can each run their own
// expect()...toX() chains without clobbering one another. The test runner
// itself (describe/test macros) is still sequential by design.
#ifdef CEST_THREAD_SAFE
#  include <pthread.h>
CEST_WEAK pthread_mutex_t _cest_mutex = PTHREAD_MUTEX_INITIALIZER;
#  define CEST_LOCK() pthread_mutex_lock(&_cest_mutex)
#  define CEST_UNLOCK() pthread_mutex_unlock(&_cest_mutex)
#  if defined(__cplusplus) && __cplusplus >= 201103L
#    define CEST_THREAD_LOCAL thread_local
#  elif defined(_CEST_C11) && _CEST_C11 && !defined(__STDC_NO_THREADS__)
#    define CEST_THREAD_LOCAL _Thread_local
#  elif defined(_MSC_VER)
#    define CEST_THREAD_LOCAL __declspec(thread)
#  elif defined(__GNUC__) || defined(__clang__)
#    define CEST_THREAD_LOCAL __thread
#  else
#    define CEST_THREAD_LOCAL
#  endif
#else
#  define CEST_LOCK()
#  define CEST_UNLOCK()
#  define CEST_THREAD_LOCAL
#endif

// Windows console color support
#ifdef _WIN32
#  ifndef CEST_NO_COLORS
#    include <windows.h>
#    define _CEST_INIT_COLORS() do { \
        DWORD mode; \
        HANDLE h = GetStdHandle(STD_OUTPUT_HANDLE); \
        if (GetConsoleMode(h, &mode)) SetConsoleMode(h, mode | ENABLE_VIRTUAL_TERMINAL_PROCESSING); \
    } while(0)
#  endif
#endif

#ifndef _CEST_INIT_COLORS
#  define _CEST_INIT_COLORS()
#endif

// Internal Math Helpers (To avoid -lm dependency)
// GNU/Clang: single evaluation via statement-expression + typeof.
// Other compilers (MSVC): falls back to the classic double-evaluation macro —
// safe today because every call site passes a side-effect-free expression,
// but avoid passing an expression with side effects (e.g. a function call) to it.
#if defined(__GNUC__) || defined(__clang__)
#  define _CEST_ABS(x) __extension__ ({ __typeof__(x) _cest_abs_v = (x); _cest_abs_v < 0 ? -_cest_abs_v : _cest_abs_v; })
#else
#  define _CEST_ABS(x) ((x) < 0 ? -(x) : (x))
#endif

// Color codes
#ifndef CEST_NO_COLORS
#  define CEST_CLR_RESET   "\033[0m"
#  define CEST_CLR_RED     "\033[31m"
#  define CEST_CLR_GREEN   "\033[32m"
#  define CEST_CLR_YELLOW  "\033[33m"
#  define CEST_CLR_BLUE    "\033[34m"
#  define CEST_CLR_BOLD    "\033[1m"
#  define CEST_CLR_DIM     "\033[90m"
#else
#  define CEST_CLR_RESET   ""
#  define CEST_CLR_RED     ""
#  define CEST_CLR_GREEN   ""
#  define CEST_CLR_YELLOW  ""
#  define CEST_CLR_BLUE    ""
#  define CEST_CLR_BOLD    ""
#  define CEST_CLR_DIM     ""
#endif

// Types and Values
typedef enum {
    CEST_TYPE_INT,
    CEST_TYPE_DOUBLE,
    CEST_TYPE_STR,
    CEST_TYPE_PTR,
    CEST_TYPE_BOOL,
    CEST_TYPE_OBJC_ID,
    CEST_TYPE_REGEX,
    CEST_TYPE_ARRAY,
    CEST_TYPE_CHAR8,
    CEST_TYPE_CHAR16,
    CEST_TYPE_CHAR32,
    CEST_TYPE_BYTE
} cest_type_t;

typedef struct {
    cest_type_t type;
    union {
        long long i;
        double d;
        const char *s;
        const void *p;
        bool b;
        unsigned char u8;
#ifdef __OBJC__
        id obj;
#endif
#ifdef __cplusplus
        std::regex* re;
#endif
    } as;
    int len;
    size_t elem_size; // arrays only: sizeof one element
} cest_value_t;

typedef int (*cest_match_fn)(cest_value_t actual, cest_value_t expected, int* diff_pos);

// ============================================================================
// Leak Detection (optional - enable with CEST_ENABLE_LEAK_DETECTION)
// ============================================================================
#ifdef CEST_ENABLE_LEAK_DETECTION
CEST_WEAK long long _cest_alloc_count = 0;
CEST_WEAK long long _cest_free_count = 0;

#define cest_malloc(size) _cest_malloc(size)
#define cest_free(ptr) _cest_free(ptr)

static inline void* _cest_malloc(size_t size) {
    void* p = malloc(size);
    if (p) _cest_alloc_count++;
    return p;
}

static inline void _cest_free(void* ptr) {
    if (ptr) _cest_free_count++;
    free(ptr);
}
#else
#define cest_malloc(size) malloc(size)
#define cest_free(ptr) free(ptr)
#endif

// ============================================================================
// Hook Identifiers
// ============================================================================
typedef enum {
    CEST_HOOK_NONE         = 0,
    CEST_HOOK_BEFORE_EACH  = 1,
    CEST_HOOK_AFTER_EACH   = 2,
    CEST_HOOK_BEFORE_ALL   = 3,
    CEST_HOOK_AFTER_ALL    = 4
} cest_hook_id_t;

// ============================================================================
// Signal Handler (crash diagnostics without fork)
// ============================================================================
#ifdef CEST_ENABLE_SIGNAL_HANDLER
static volatile const char* _cest_signal_test_name = NULL;
static volatile int _cest_in_hook = CEST_HOOK_NONE;

static const char* _cest_signal_name(int sig) {
    switch(sig) {
        case SIGSEGV: return "SIGSEGV (Segmentation Fault)";
        case SIGABRT: return "SIGABRT (Abort)";
        case SIGFPE:  return "SIGFPE (Floating Point Exception)";
        case SIGBUS:  return "SIGBUS (Bus Error)";
        case SIGILL:  return "SIGILL (Illegal Instruction)";
        default:      return "Unknown Signal";
    }
}

static const char* _cest_hook_name(int hook_id) {
    switch(hook_id) {
        case CEST_HOOK_BEFORE_EACH: return "beforeEach";
        case CEST_HOOK_AFTER_EACH:  return "afterEach";
        case CEST_HOOK_BEFORE_ALL:  return "beforeAll";
        case CEST_HOOK_AFTER_ALL:   return "afterAll";
        default: return "unknown hook";
    }
}

static void _cest_signal_handler(int sig) {
    // Use write() instead of printf() — async-signal-safe
    const char* msg_prefix = "\n  \033[31m✕ CRASH: ";
    const char* msg_signal = _cest_signal_name(sig);
    const char* msg_reset = "\033[0m\n";
    (void)!write(STDERR_FILENO, msg_prefix, strlen(msg_prefix));
    (void)!write(STDERR_FILENO, msg_signal, strlen(msg_signal));
    if (_cest_in_hook) {
        const char* hook_msg = " in hook: ";
        const char* hook_name = _cest_hook_name(_cest_in_hook);
        (void)!write(STDERR_FILENO, hook_msg, strlen(hook_msg));
        (void)!write(STDERR_FILENO, hook_name, strlen(hook_name));
    }
    if (_cest_signal_test_name) {
        const char* test_msg = " during test: ";
        (void)!write(STDERR_FILENO, test_msg, strlen(test_msg));
        (void)!write(STDERR_FILENO, (const char*)_cest_signal_test_name, strlen((const char*)_cest_signal_test_name));
    }
    (void)!write(STDERR_FILENO, msg_reset, strlen(msg_reset));
    _exit(128 + sig);
}

static inline void _cest_install_signal_handlers(void) {
    struct sigaction sa;
    memset(&sa, 0, sizeof(sa));
    sa.sa_handler = _cest_signal_handler;
    sigemptyset(&sa.sa_mask);
    sa.sa_flags = SA_RESETHAND; // One-shot: restore default after first signal
    sigaction(SIGSEGV, &sa, NULL);
    sigaction(SIGABRT, &sa, NULL);
    sigaction(SIGFPE,  &sa, NULL);
    sigaction(SIGBUS,  &sa, NULL);
    sigaction(SIGILL,  &sa, NULL);
}
#  define _CEST_SIGNAL_INIT()         _cest_install_signal_handlers()
#  define _CEST_SIGNAL_SET_TEST(name) (_cest_signal_test_name = (name))
#  define _CEST_SIGNAL_SET_HOOK(id)   (_cest_in_hook = (id))
#  define _CEST_SIGNAL_CLEAR_HOOK()   (_cest_in_hook = CEST_HOOK_NONE)
#else
#  define _CEST_SIGNAL_INIT()
#  define _CEST_SIGNAL_SET_TEST(name) ((void)(name))
#  define _CEST_SIGNAL_SET_HOOK(id)   ((void)(id))
#  define _CEST_SIGNAL_CLEAR_HOOK()
#endif

// ============================================================================
// Stats and Globals
// ============================================================================
typedef struct {
    int passed;
    int failed;
    int skipped;
    const char* filter_pattern;
} cest_stats_t;

CEST_WEAK cest_stats_t _cest_global_stats = {0, 0, 0, NULL};

// Public, prefixed accessor for the shared stats counters. _cest_global_stats
// was documented as the public way to read pass/fail/skip counts despite its
// leading underscore (an internal-naming convention elsewhere in this file),
// which is inconsistent and makes it look like an implementation detail
// that should not be touched directly. Prefer cest_stats() in new code;
// _cest_global_stats is kept, unchanged, for backward compatibility.
static inline cest_stats_t cest_stats(void) { return _cest_global_stats; }

CEST_WEAK const char* _cest_current_test_name = NULL;
CEST_WEAK const char* _cest_junit_output = NULL;
CEST_WEAK const char* _cest_json_output = NULL;
CEST_WEAK double _cest_total_time = 0.0;
CEST_WEAK clock_t _cest_suite_start_time = 0;

// ============================================================================
// Per-test recording (used by --junit / --json reports)
// ============================================================================
#ifndef CEST_MAX_TEST_RECORDS
#  define CEST_MAX_TEST_RECORDS 4096
#endif

typedef struct {
    const char* name;
    double time;
    int failed;  // 1 if at least one assertion failed during this test
    int skipped; // 1 if this test was filtered out and never ran
} cest_test_record_t;

CEST_WEAK cest_test_record_t _cest_test_records[CEST_MAX_TEST_RECORDS] = { {NULL, 0, 0, 0} };
CEST_WEAK int _cest_test_record_count = 0;

static inline void _cest_record_test(const char* name, double time, int failed, int skipped) {
    CEST_LOCK();
    if (_cest_test_record_count < CEST_MAX_TEST_RECORDS) {
        _cest_test_records[_cest_test_record_count].name = name;
        _cest_test_records[_cest_test_record_count].time = time;
        _cest_test_records[_cest_test_record_count].failed = failed;
        _cest_test_records[_cest_test_record_count].skipped = skipped;
        _cest_test_record_count++;
    }
    CEST_UNLOCK();
}

// Minimal escaping so test names never produce malformed reports.
static inline void _cest_fputs_xml_escaped(const char* s, FILE* f) {
    if (!s) return;
    for (const char* p = s; *p; p++) {
        switch (*p) {
            case '&':  fputs("&amp;", f);  break;
            case '<':  fputs("&lt;", f);   break;
            case '>':  fputs("&gt;", f);   break;
            case '"':  fputs("&quot;", f); break;
            case '\'': fputs("&apos;", f); break;
            default:   fputc(*p, f);
        }
    }
}

static inline void _cest_fputs_json_escaped(const char* s, FILE* f) {
    if (!s) return;
    for (const char* p = s; *p; p++) {
        switch (*p) {
            case '"':  fputs("\\\"", f); break;
            case '\\': fputs("\\\\", f); break;
            case '\n': fputs("\\n", f);  break;
            case '\t': fputs("\\t", f);  break;
            default:
                if ((unsigned char)*p < 0x20) fprintf(f, "\\u%04x", *p);
                else fputc(*p, f);
        }
    }
}

// ============================================================================
// Coverage Support
// ============================================================================
#ifdef CEST_ENABLE_COVERAGE
#  define CEST_COVERAGE_INIT() \
    do { \
        extern void __gcov_flush(void); \
        atexit(__gcov_flush); \
    } while(0)
#  define _CEST_COVERAGE_BEFORE_EACH() \
    do { \
        extern void __gcov_reset(void); \
        __gcov_reset(); \
    } while(0)
#  define _CEST_COVERAGE_AFTER_EACH() \
    do { \
        extern void __gcov_dump(void); \
        __gcov_dump(); \
    } while(0)
#else
#  define CEST_COVERAGE_INIT()
#  define _CEST_COVERAGE_BEFORE_EACH()
#  define _CEST_COVERAGE_AFTER_EACH()
#endif

// ============================================================================
// Value Wrappers
// ============================================================================
static inline cest_value_t cest_int(long long v) { cest_value_t cv; cv.type = CEST_TYPE_INT; cv.as.i = v; cv.len = 0; return cv; }
static inline cest_value_t cest_double(double v) { cest_value_t cv; cv.type = CEST_TYPE_DOUBLE; cv.as.d = v; cv.len = 0; return cv; }
// Keeps a NULL char*/const char* as an actual NULL instead of the literal
// string "NULL", so toBeNull() and the null-safe matchers below can tell a
// missing string apart from the four-character text "NULL".
static inline cest_value_t cest_str(const char* v) { cest_value_t cv; cv.type = CEST_TYPE_STR; cv.as.s = v; cv.len = 0; return cv; }
static inline cest_value_t cest_ptr(const void* v) { cest_value_t cv; cv.type = CEST_TYPE_PTR; cv.as.p = v; cv.len = 0; return cv; }
static inline cest_value_t cest_bool(bool v) { cest_value_t cv; cv.type = CEST_TYPE_BOOL; cv.as.b = v; cv.len = 0; return cv; }
static inline cest_value_t _cest_array(const void* v, int len, size_t elem_size) { cest_value_t cv; cv.type = CEST_TYPE_ARRAY; cv.as.p = v; cv.len = len; cv.elem_size = elem_size; return cv; }
#define cest_array(x, len) _cest_array((x), (len), sizeof(*(x)))
#ifdef __OBJC__
static inline cest_value_t cest_id(id v) { cest_value_t cv; cv.type = CEST_TYPE_OBJC_ID; cv.as.obj = v; cv.len = 0; return cv; }
#endif

#ifdef __cplusplus
static inline cest_value_t cest_value(bool v) { return cest_bool(v); }
static inline cest_value_t cest_value(int v) { return cest_int(v); }
static inline cest_value_t cest_value(long v) { return cest_int(v); }
static inline cest_value_t cest_value(long long v) { return cest_int(v); }
static inline cest_value_t cest_value(float v) { return cest_double(v); }
static inline cest_value_t cest_value(double v) { return cest_double(v); }
static inline cest_value_t cest_value(char* v) { return cest_str(v); }
static inline cest_value_t cest_value(const char* v) { return cest_str(v); }
static inline cest_value_t cest_value(const std::string& v) { return cest_str(v.c_str()); }
#ifdef __OBJC__
static inline cest_value_t cest_value(id v) { return cest_id(v); }
#endif
template<typename T>
static inline cest_value_t cest_value(T* v) { return cest_ptr((const void*)v); }
#else
#define cest_value(x) \
    _Generic((x), \
        _Bool: cest_bool, \
        int: cest_int, \
        long: cest_int, \
        long long: cest_int, \
        float: cest_double, \
        double: cest_double, \
        char*: cest_str, \
        const char*: cest_str, \
        _CEST_OBJC_GENERIC(x) \
        default: cest_ptr \
    )(x)

#ifdef __OBJC__
#  define _CEST_OBJC_GENERIC(x) id: cest_id,
#else
#  define _CEST_OBJC_GENERIC(x)
#endif
#endif

// ============================================================================
// Core Assertion Implementation
// ============================================================================
static CEST_THREAD_LOCAL struct {
    const char* file;
    int line;
    const char* actual_expr;
    cest_value_t actual;
    cest_value_t expected;
    const char* expected_expr;
} _cest_ctx;

// Reset context to a clean state — prevents stale data leaking between asserts
static inline void _cest_ctx_reset(void) {
    _cest_ctx.file = NULL;
    _cest_ctx.line = 0;
    _cest_ctx.actual_expr = NULL;
    memset(&_cest_ctx.actual, 0, sizeof(cest_value_t));
    memset(&_cest_ctx.expected, 0, sizeof(cest_value_t));
    _cest_ctx.expected_expr = NULL;
}

static inline void _cest_print_value(cest_value_t v) {
    switch(v.type) {
        case CEST_TYPE_INT: printf("(int: %lld)", v.as.i); break;
        case CEST_TYPE_DOUBLE: printf("(double: %g)", v.as.d); break;
        case CEST_TYPE_STR:
            if (v.as.s) printf("(string: \"%s\")", v.as.s);
            else printf("(string: null)");
            break;
        case CEST_TYPE_PTR: printf("(pointer: %p)", v.as.p); break;
        case CEST_TYPE_BOOL: printf("(bool: %s)", v.as.b ? "true" : "false"); break;
        case CEST_TYPE_ARRAY: printf("(array[%d]: %p)", v.len, v.as.p); break;
#ifdef __OBJC__
        case CEST_TYPE_OBJC_ID: printf("(id: %p)", (void*)v.as.obj); break;
#else
        case CEST_TYPE_OBJC_ID: printf("(id: %p)", v.as.p); break;
#endif
        case CEST_TYPE_REGEX: printf("(regex)"); break;
        default: printf("(unknown)"); break;
    }
}

static inline void _cest_assert_impl(cest_value_t expected, cest_match_fn match, const char* match_name, const char* expected_expr) {
    int diff_pos = -1;
    int passed = match(_cest_ctx.actual, expected, &diff_pos);
    CEST_LOCK();
    if (passed) {
        printf("  " CEST_CLR_GREEN "✓" CEST_CLR_RESET " %s %s %s\n", _cest_ctx.actual_expr, match_name, expected_expr);
        _cest_global_stats.passed++;
    } else {
        printf("  " CEST_CLR_RED "✕ %s failed" CEST_CLR_RESET "\n", _cest_ctx.actual_expr);
        printf("    " CEST_CLR_GREEN "Expected %s: %s" CEST_CLR_RESET "\n", match_name, expected_expr);
        printf("    " CEST_CLR_RED "Received "); _cest_print_value(_cest_ctx.actual); printf(CEST_CLR_RESET "\n");
        if (_cest_ctx.actual.type == CEST_TYPE_STR && expected.type == CEST_TYPE_STR) {
            if (diff_pos >= 0) {
                printf("    " CEST_CLR_DIM "Diff at position %d: '%c' vs '%c'" CEST_CLR_RESET "\n", 
                       diff_pos, _cest_ctx.actual.as.s[diff_pos], expected.as.s[diff_pos]);
            }
        }
        if (_cest_ctx.actual.type == CEST_TYPE_DOUBLE && expected.type == CEST_TYPE_DOUBLE) {
            double diff = _CEST_ABS(_cest_ctx.actual.as.d - expected.as.d);
            printf("    " CEST_CLR_DIM "Diff: %g" CEST_CLR_RESET "\n", diff);
        }
        if (_cest_ctx.actual.type == CEST_TYPE_ARRAY && expected.type == CEST_TYPE_ARRAY) {
            if (_cest_ctx.actual.len != expected.len) {
                printf("    " CEST_CLR_DIM "Length mismatch: %d vs %d" CEST_CLR_RESET "\n", _cest_ctx.actual.len, expected.len);
            } else {
                printf("    " CEST_CLR_DIM "Arrays differ in content" CEST_CLR_RESET "\n");
            }
        }
        printf("    " CEST_CLR_DIM "(%s:%d)" CEST_CLR_RESET "\n", _cest_ctx.file, _cest_ctx.line);
        _cest_global_stats.failed++;
    }
    fflush(stdout); // Ensure output is visible even if test crashes after this assert
    CEST_UNLOCK();
}

// ============================================================================
// Matcher Logic
// ============================================================================
static inline int match_eq(cest_value_t a, cest_value_t b, int* diff_pos) {
    if (diff_pos) *diff_pos = -1;
    if (a.type != b.type) {
        // toBeNull() builds its expectation as cest_ptr(NULL); a NULL
        // char*/const char* actual represents the same "no value" and must
        // be able to match it even though the stored types differ.
        if ((a.type == CEST_TYPE_STR && a.as.s == NULL && b.type == CEST_TYPE_PTR && b.as.p == NULL) ||
            (b.type == CEST_TYPE_STR && b.as.s == NULL && a.type == CEST_TYPE_PTR && a.as.p == NULL)) {
            return 1;
        }
#ifdef __OBJC__
        if ((a.type == CEST_TYPE_PTR && b.type == CEST_TYPE_OBJC_ID) ||
            (a.type == CEST_TYPE_OBJC_ID && b.type == CEST_TYPE_PTR)) {
            const void *p = a.type == CEST_TYPE_PTR ? a.as.p : b.as.p;
            id obj = a.type == CEST_TYPE_OBJC_ID ? a.as.obj : b.as.obj;
            return p == (const void*)obj;
        }
        if ((a.type == CEST_TYPE_PTR || a.type == CEST_TYPE_OBJC_ID) && 
            (b.type == CEST_TYPE_PTR || b.type == CEST_TYPE_OBJC_ID)) {
            const void *p1 = a.type == CEST_TYPE_PTR ? a.as.p : (const void*)a.as.obj;
            const void *p2 = b.type == CEST_TYPE_PTR ? b.as.p : (const void*)b.as.obj;
            return p1 == p2;
        }
#endif
        return 0;
    }
    switch (a.type) {
        case CEST_TYPE_INT: return a.as.i == b.as.i;
        case CEST_TYPE_DOUBLE: return _CEST_ABS(a.as.d - b.as.d) < 0.000001;
        case CEST_TYPE_STR: {
            if (a.as.s == NULL || b.as.s == NULL) return a.as.s == b.as.s;
            if (strcmp(a.as.s, b.as.s) == 0) return 1;
            if (diff_pos) {
                const char* p1 = a.as.s;
                const char* p2 = b.as.s;
                while (*p1 && *p2) {
                    if (*p1 != *p2) break;
                    p1++; p2++;
                }
                *diff_pos = (int)(p1 - a.as.s);
            }
            return 0;
        }
        case CEST_TYPE_PTR: return a.as.p == b.as.p;
        case CEST_TYPE_BOOL: return a.as.b == b.as.b;
        case CEST_TYPE_ARRAY: {
            if (a.len != b.len || a.elem_size != b.elem_size) return 0;
            if (a.len == 0) return 1;
            return memcmp(a.as.p, b.as.p, (size_t)a.len * a.elem_size) == 0;
        }
#ifdef __OBJC__
        case CEST_TYPE_OBJC_ID: return a.as.obj == b.as.obj;
#endif
        default: return 0;
    }
}

static inline int match_gt(cest_value_t a, cest_value_t b, int* diff_pos) {
    (void)diff_pos;
    if (a.type == CEST_TYPE_INT && b.type == CEST_TYPE_INT) return a.as.i > b.as.i;
    if (a.type == CEST_TYPE_DOUBLE && b.type == CEST_TYPE_DOUBLE) return a.as.d > b.as.d;
    return 0;
}

static inline int match_lt(cest_value_t a, cest_value_t b, int* diff_pos) {
    (void)diff_pos;
    if (a.type == CEST_TYPE_INT && b.type == CEST_TYPE_INT) return a.as.i < b.as.i;
    if (a.type == CEST_TYPE_DOUBLE && b.type == CEST_TYPE_DOUBLE) return a.as.d < b.as.d;
    return 0;
}

static inline int match_in_range(cest_value_t a, cest_value_t min, cest_value_t max, int* diff_pos) {
    (void)diff_pos;
    if (a.type == CEST_TYPE_INT && min.type == CEST_TYPE_INT && max.type == CEST_TYPE_INT) {
        return a.as.i >= min.as.i && a.as.i <= max.as.i;
    }
    if (a.type == CEST_TYPE_DOUBLE && min.type == CEST_TYPE_DOUBLE && max.type == CEST_TYPE_DOUBLE) {
        return a.as.d >= min.as.d && a.as.d <= max.as.d;
    }
    return 0;
}

static inline int match_start_with(cest_value_t a, cest_value_t b, int* diff_pos) {
    (void)diff_pos;
    if (a.type == CEST_TYPE_STR && b.type == CEST_TYPE_STR) {
        if (a.as.s == NULL || b.as.s == NULL) return 0;
        return strncmp(a.as.s, b.as.s, strlen(b.as.s)) == 0;
    }
    return 0;
}

static inline int match_end_with(cest_value_t a, cest_value_t b, int* diff_pos) {
    (void)diff_pos;
    if (a.type == CEST_TYPE_STR && b.type == CEST_TYPE_STR) {
        if (a.as.s == NULL || b.as.s == NULL) return 0;
        size_t len_a = strlen(a.as.s);
        size_t len_b = strlen(b.as.s);
        if (len_b > len_a) return 0;
        return strcmp(a.as.s + len_a - len_b, b.as.s) == 0;
    }
    return 0;
}

static inline int match_contain(cest_value_t a, cest_value_t b, int* diff_pos) {
    (void)diff_pos;
    if (a.type == CEST_TYPE_STR && b.type == CEST_TYPE_STR) {
        if (a.as.s == NULL || b.as.s == NULL) return 0;
        return strstr(a.as.s, b.as.s) != NULL;
    }
    return 0;
}

static inline int match_regex(cest_value_t a, cest_value_t b, int* diff_pos) {
#ifdef __cplusplus
    (void)diff_pos;
    if (a.type == CEST_TYPE_STR && b.type == CEST_TYPE_REGEX) {
        if (a.as.s == NULL) return 0;
        return std::regex_match(a.as.s, *b.as.re);
    }
#else
    (void)a; (void)b; (void)diff_pos;
#endif
    return 0;
}

static inline int match_truthy(cest_value_t a, cest_value_t b, int* diff_pos) {
    (void)b; (void)diff_pos;
    switch (a.type) {
        case CEST_TYPE_BOOL:   return a.as.b;
        case CEST_TYPE_INT:    return a.as.i != 0;
        case CEST_TYPE_DOUBLE: return a.as.d != 0.0;
        case CEST_TYPE_STR:    return a.as.s != NULL && strlen(a.as.s) > 0;
        case CEST_TYPE_PTR:    return a.as.p != NULL;
#ifdef __OBJC__
        case CEST_TYPE_OBJC_ID: return a.as.obj != nil;
#endif
        default: return 0;
    }
}

static inline int match_falsy(cest_value_t a, cest_value_t b, int* diff_pos) {
    return !match_truthy(a, b, diff_pos);
}

static inline int match_defined(cest_value_t a, cest_value_t b, int* diff_pos) {
    (void)b; (void)diff_pos;
    if (a.type == CEST_TYPE_PTR && a.as.p == NULL) return 0;
#ifdef __OBJC__
    if (a.type == CEST_TYPE_OBJC_ID && a.as.obj == nil) return 0;
#endif
    return 1;
}

static inline int match_undefined(cest_value_t a, cest_value_t b, int* diff_pos) {
    return !match_defined(a, b, diff_pos);
}

// ============================================================================
// Fluent API Bridge
// ============================================================================
typedef struct {
    void (*_toEqual)(cest_value_t expected, const char* expected_expr);
    void (*_toBe)(cest_value_t expected, const char* expected_expr);
    void (*_toBeGreaterThan)(cest_value_t expected, const char* expected_expr);
    void (*_toBeLessThan)(cest_value_t expected, const char* expected_expr);
    void (*_toContain)(cest_value_t expected, const char* expected_expr);
    void (*_toBeInRange)(cest_value_t min, cest_value_t max, const char* range_expr);
    void (*_toStartWith)(cest_value_t expected, const char* expected_expr);
    void (*_toEndWith)(cest_value_t expected, const char* expected_expr);
    void (*_toBeNull)(void);
    void (*_toBeTruthy)(void);
    void (*_toBeFalsy)(void);
    void (*_toBeCloseTo)(double val, double precision);
    void (*_toEqualArray)(cest_value_t expected, const char* expected_expr);
    void (*_toMatch)(cest_value_t expected, const char* expected_expr);
    void (*_toBeDefined)(void);
    void (*_toBeUndefined)(void);
} _cest_bridge_t;

static inline void b_toEqual(cest_value_t e, const char* ee) { _cest_assert_impl(e, match_eq, "to equal", ee); }
static inline void b_toEqualArray(cest_value_t e, const char* ee) { _cest_assert_impl(e, match_eq, "to equal array", ee); }
static inline void b_toBeGreaterThan(cest_value_t e, const char* ee) { _cest_assert_impl(e, match_gt, "to be greater than", ee); }
static inline void b_toBeLessThan(cest_value_t e, const char* ee) { _cest_assert_impl(e, match_lt, "to be less than", ee); }
static inline void b_toContain(cest_value_t e, const char* ee) { _cest_assert_impl(e, match_contain, "to contain", ee); }
static inline void b_toStartWith(cest_value_t e, const char* ee) { _cest_assert_impl(e, match_start_with, "to start with", ee); }
static inline void b_toEndWith(cest_value_t e, const char* ee) { _cest_assert_impl(e, match_end_with, "to end with", ee); }
static inline void b_toMatch(cest_value_t e, const char* ee) { _cest_assert_impl(e, match_regex, "to match", ee); }
static inline void b_toBeNull(void) { _cest_assert_impl(cest_ptr(NULL), match_eq, "to be null", "NULL"); }
static inline void b_toBeTruthy(void) { _cest_assert_impl(cest_bool(true), match_truthy, "to be", "truthy"); }
static inline void b_toBeFalsy(void) { _cest_assert_impl(cest_bool(false), match_falsy, "to be", "falsy"); }
static inline void b_toBeDefined(void) { _cest_assert_impl(cest_bool(true), match_defined, "to be", "defined"); }
static inline void b_toBeUndefined(void) { _cest_assert_impl(cest_bool(true), match_undefined, "to be", "undefined"); }
static inline void b_toBeCloseTo(double val, double precision) {
    cest_value_t actual = _cest_ctx.actual;
    int numeric = actual.type == CEST_TYPE_DOUBLE || actual.type == CEST_TYPE_INT;
    double actual_d = actual.type == CEST_TYPE_INT ? (double)actual.as.i : actual.as.d;
    double diff = numeric ? _CEST_ABS(actual_d - val) : 0.0;
    CEST_LOCK();
    if (numeric && diff < precision) {
        printf("  " CEST_CLR_GREEN "✓" CEST_CLR_RESET " %s to be close to %g (precision %g)\n", _cest_ctx.actual_expr, val, precision);
        _cest_global_stats.passed++;
    } else {
        printf("  " CEST_CLR_RED "✕ %s to be close to" CEST_CLR_RESET "\n", _cest_ctx.actual_expr);
        printf("    " CEST_CLR_GREEN "Expected to be close to: %g (precision %g)" CEST_CLR_RESET "\n", val, precision);
        printf("    " CEST_CLR_RED "Received "); _cest_print_value(actual);
        if (numeric) printf(" (diff %g)", diff);
        printf(CEST_CLR_RESET "\n");
        printf("    " CEST_CLR_DIM "(%s:%d)" CEST_CLR_RESET "\n", _cest_ctx.file, _cest_ctx.line);
        _cest_global_stats.failed++;
    }
    fflush(stdout);
    CEST_UNLOCK();
}
static inline void b_toBeInRange(cest_value_t min, cest_value_t max, const char* re) {
    int passed = match_in_range(_cest_ctx.actual, min, max, NULL);
    CEST_LOCK();
    if (passed) {
        printf("  " CEST_CLR_GREEN "✓" CEST_CLR_RESET " %s to be in range %s\n", _cest_ctx.actual_expr, re);
        _cest_global_stats.passed++;
    } else {
        printf("  " CEST_CLR_RED "✕ %s to be in range" CEST_CLR_RESET "\n", _cest_ctx.actual_expr);
        printf("    " CEST_CLR_GREEN "Expected range: %s" CEST_CLR_RESET "\n", re);
        printf("    " CEST_CLR_RED "Received "); _cest_print_value(_cest_ctx.actual); printf(CEST_CLR_RESET "\n");
        printf("    " CEST_CLR_DIM "(%s:%d)" CEST_CLR_RESET "\n", _cest_ctx.file, _cest_ctx.line);
        _cest_global_stats.failed++;
    }
    fflush(stdout);
    CEST_UNLOCK();
}

// Positional (not designated) initialization: designated initializers
// require C++20 on MSVC, but this struct is also used from C++11/14/17
// translation units. Field order here must match _cest_bridge_t exactly.
static _cest_bridge_t _cest_bridge CEST_UNUSED = {
    b_toEqual,
    b_toEqual,
    b_toBeGreaterThan,
    b_toBeLessThan,
    b_toContain,
    b_toBeInRange,
    b_toStartWith,
    b_toEndWith,
    b_toBeNull,
    b_toBeTruthy,
    b_toBeFalsy,
    b_toBeCloseTo,
    b_toEqualArray,
    b_toMatch,
    b_toBeDefined,
    b_toBeUndefined
};

#define expect(x) (_cest_ctx_reset(), _cest_ctx.file = __FILE__, _cest_ctx.line = __LINE__, _cest_ctx.actual_expr = #x, _cest_ctx.actual = cest_value(x), _cest_bridge)
#define expect_array(x, len) (_cest_ctx_reset(), _cest_ctx.file = __FILE__, _cest_ctx.line = __LINE__, _cest_ctx.actual_expr = #x, _cest_ctx.actual = cest_array(x, len), _cest_bridge)

#define toEqual(x) _toEqual(cest_value(x), #x)
#define toBe(x) _toBe(cest_value(x), #x)
#define toBeGreaterThan(x) _toBeGreaterThan(cest_value(x), #x)
#define toBeLessThan(x) _toBeLessThan(cest_value(x), #x)
#define toContain(x) _toContain(cest_value(x), #x)
#define toBeInRange(min, max) _toBeInRange(cest_value(min), cest_value(max), #min " to " #max)
#define toStartWith(x) _toStartWith(cest_value(x), #x)
#define toEndWith(x) _toEndWith(cest_value(x), #x)
#define toBeNull() _toBeNull()
#define toBeTruthy() _toBeTruthy()
#define toBeFalsy() _toBeFalsy()
#define toBeCloseTo(v, p) _toBeCloseTo(v, p)
#define toEqualArray(x, len) _toEqualArray(cest_array(x, len), #x ", " #len)
#define toMatch(regex) _toMatch(cest_value(regex), #regex)
#define toBeDefined() _toBeDefined()
#define toBeUndefined() _toBeUndefined()

// ============================================================================
// Test Runner Macros with Timing
// ============================================================================
static clock_t _cest_test_start_time CEST_UNUSED = 0;

#define describe(name, block) do { \
    printf("\n" CEST_CLR_BOLD "● %s" CEST_CLR_RESET "\n", name); \
    fflush(stdout); \
    _CEST_RUN_BEFORE_ALL(); \
    block \
    _CEST_RUN_AFTER_ALL(); \
} while (0)

#define test(name, block) do { \
    if (!_cest_should_run_test(name)) { \
        printf("  " CEST_CLR_DIM "○ %s (filtered)" CEST_CLR_RESET "\n", name); \
        CEST_LOCK(); _cest_global_stats.skipped++; CEST_UNLOCK(); \
        _cest_record_test(name, 0.0, 0, 1); \
    } else { \
        _cest_current_test_name = name; \
        _CEST_SIGNAL_SET_TEST(name); \
        _cest_test_start_time = clock(); \
        int _cest_failed_before = _cest_global_stats.failed; \
        printf("  %s\n", name); \
        fflush(stdout); \
        _CEST_RUN_BEFORE_EACH(); \
        block; \
        _CEST_RUN_AFTER_EACH(); \
        fflush(stdout); \
        _cest_record_test(name, (double)(clock() - _cest_test_start_time) / CLOCKS_PER_SEC, _cest_global_stats.failed > _cest_failed_before, 0); \
    } \
} while (0)

#define bench(name, block) do { \
    clock_t _bench_start = clock(); \
    for(int _i = 0; _i < 1000; _i++) { block } \
    clock_t _bench_end = clock(); \
    double _bench_time = (double)(_bench_end - _bench_start) / CLOCKS_PER_SEC; \
    printf("  " CEST_CLR_BLUE "⚡ %s: %.6fs total, %.6fs avg" CEST_CLR_RESET "\n", name, _bench_time, _bench_time / 1000); \
} while (0)

#define it(name, block) test(name, block)

// ============================================================================
// Hooks
// ============================================================================
#ifndef CEST_NO_HOOKS
typedef void (*cest_hook_fn)(void);
static cest_hook_fn _cest_before_each_fn CEST_UNUSED = NULL;
static cest_hook_fn _cest_after_each_fn CEST_UNUSED = NULL;
static cest_hook_fn _cest_before_all_fn CEST_UNUSED = NULL;
static cest_hook_fn _cest_after_all_fn CEST_UNUSED = NULL;

#define beforeEach(fn) do { _cest_before_each_fn = fn; } while (0)
#define afterEach(fn) do { _cest_after_each_fn = fn; } while (0)
#define beforeAll(fn) do { _cest_before_all_fn = fn; } while (0)
#define afterAll(fn) do { _cest_after_all_fn = fn; } while (0)

#define _CEST_RUN_BEFORE_EACH() do { \
    _CEST_SIGNAL_SET_HOOK(CEST_HOOK_BEFORE_EACH); \
    if (_cest_before_each_fn) _cest_before_each_fn(); \
    _CEST_SIGNAL_CLEAR_HOOK(); \
    _CEST_COVERAGE_BEFORE_EACH(); \
} while(0)
#define _CEST_RUN_AFTER_EACH() do { \
    _CEST_SIGNAL_SET_HOOK(CEST_HOOK_AFTER_EACH); \
    if (_cest_after_each_fn) _cest_after_each_fn(); \
    _CEST_SIGNAL_CLEAR_HOOK(); \
    _CEST_COVERAGE_AFTER_EACH(); \
} while(0)
#define _CEST_RUN_BEFORE_ALL() do { \
    _CEST_SIGNAL_SET_HOOK(CEST_HOOK_BEFORE_ALL); \
    if (_cest_before_all_fn) _cest_before_all_fn(); \
    _CEST_SIGNAL_CLEAR_HOOK(); \
} while(0)
#define _CEST_RUN_AFTER_ALL() do { \
    _CEST_SIGNAL_SET_HOOK(CEST_HOOK_AFTER_ALL); \
    if (_cest_after_all_fn) _cest_after_all_fn(); \
    _CEST_SIGNAL_CLEAR_HOOK(); \
} while(0)
#else
#define beforeEach(fn)
#define afterEach(fn)
#define beforeAll(fn)
#define afterAll(fn)
#define _CEST_RUN_BEFORE_EACH()
#define _CEST_RUN_AFTER_EACH()
#define _CEST_RUN_BEFORE_ALL()
#define _CEST_RUN_AFTER_ALL()
#endif

// ============================================================================
// CLI Arguments and Reports
// ============================================================================
#ifndef CEST_NO_CLI
static inline void _cest_write_junit(void) {
    if (!_cest_junit_output) return;
    FILE* f = fopen(_cest_junit_output, "w");
    if (!f) return;
    // tests/failures must count testcase elements (one per it()/test(), plus
    // one per filtered-out test), not assertions: a test can hold several
    // expect() calls, and _cest_global_stats.passed/failed count those, not
    // the tests that contain them.
    int tests = _cest_test_record_count;
    int failures = 0;
    for (int i = 0; i < _cest_test_record_count; i++) {
        if (_cest_test_records[i].failed) failures++;
    }
    fprintf(f, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n");
    fprintf(f, "<testsuites tests=\"%d\" failures=\"%d\" skipped=\"%d\" time=\"%.3f\">\n",
            tests, failures, _cest_global_stats.skipped, _cest_total_time);
    fprintf(f, "  <testsuite name=\"Cest\" tests=\"%d\" failures=\"%d\" skipped=\"%d\" time=\"%.3f\">\n",
            tests, failures, _cest_global_stats.skipped, _cest_total_time);
    for (int i = 0; i < _cest_test_record_count; i++) {
        fprintf(f, "    <testcase name=\"");
        _cest_fputs_xml_escaped(_cest_test_records[i].name, f);
        fprintf(f, "\" time=\"%.6f\">", _cest_test_records[i].time);
        if (_cest_test_records[i].skipped) {
            fprintf(f, "<skipped/>");
        } else if (_cest_test_records[i].failed) {
            fprintf(f, "<failure message=\"assertion failed\"></failure>");
        }
        fprintf(f, "</testcase>\n");
    }
    fprintf(f, "  </testsuite>\n");
    fprintf(f, "</testsuites>\n");
    fclose(f);
}

static inline void _cest_write_json(void) {
    if (!_cest_json_output) return;
    FILE* f = fopen(_cest_json_output, "w");
    if (!f) return;
    fprintf(f, "{\n");
    fprintf(f, "  \"stats\": {\n");
    fprintf(f, "    \"passed\": %d,\n", _cest_global_stats.passed);
    fprintf(f, "    \"failed\": %d,\n", _cest_global_stats.failed);
    fprintf(f, "    \"skipped\": %d,\n", _cest_global_stats.skipped);
    fprintf(f, "    \"total_time\": %.3f\n", _cest_total_time);
    fprintf(f, "  },\n");
    fprintf(f, "  \"tests\": [\n");
    for (int i = 0; i < _cest_test_record_count; i++) {
        fprintf(f, "    {\"name\": \"");
        _cest_fputs_json_escaped(_cest_test_records[i].name, f);
        fprintf(f, "\", \"time\": %.6f, \"status\": \"%s\"}%s\n",
                _cest_test_records[i].time,
                _cest_test_records[i].skipped ? "skipped" : (_cest_test_records[i].failed ? "failed" : "passed"),
                (i + 1 < _cest_test_record_count) ? "," : "");
    }
    fprintf(f, "  ]\n");
    fprintf(f, "}\n");
    fclose(f);
}

static inline void _cest_parse_cli_args(int argc, char* argv[]) {
    _CEST_INIT_COLORS();

    for (int i = 1; i < argc; i++) {
        if (argv[i][0] == '-') {
            if (strcmp(argv[i], "--junit") == 0 && i + 1 < argc) {
                _cest_junit_output = argv[++i];
            } else if (strcmp(argv[i], "--json") == 0 && i + 1 < argc) {
                _cest_json_output = argv[++i];
            }
        } else {
            _cest_global_stats.filter_pattern = argv[i];
        }
    }
}

static inline int _cest_should_run_test(const char* name) {
    if (!_cest_global_stats.filter_pattern) return 1;
    return strstr(name, _cest_global_stats.filter_pattern) != NULL;
}

#define cest_init(argc, argv) do { _cest_parse_cli_args(argc, argv); _CEST_SIGNAL_INIT(); } while(0)
#else
#define cest_init(argc, argv) do { _CEST_SIGNAL_INIT(); } while(0)
#define _cest_should_run_test(name) (1)
// --junit/--json are CLI flags; with CEST_NO_CLI there's no way to set the
// output path, so these are no-ops (cest_result() still calls them unconditionally).
static inline void _cest_write_junit(void) {}
static inline void _cest_write_json(void) {}
#endif

// ============================================================================
// Enhanced Results
// ============================================================================
static inline int cest_result(void) {
    _cest_total_time = (double)(clock() - _cest_suite_start_time) / CLOCKS_PER_SEC;
    
    printf("\n" CEST_CLR_BOLD "Test Suites Summary:" CEST_CLR_RESET "\n");
    if (_cest_global_stats.failed == 0) {
        printf("  " CEST_CLR_GREEN "All %d tests passed!" CEST_CLR_RESET, _cest_global_stats.passed);
        if (_cest_global_stats.skipped > 0) printf(" (" CEST_CLR_YELLOW "%d skipped" CEST_CLR_RESET ")", _cest_global_stats.skipped);
        printf("\n");
    } else {
        printf("  " CEST_CLR_GREEN "Passed: %d" CEST_CLR_RESET, _cest_global_stats.passed);
        printf("\n  " CEST_CLR_RED "Failed: %d" CEST_CLR_RESET, _cest_global_stats.failed);
        if (_cest_global_stats.skipped > 0) printf("\n  " CEST_CLR_YELLOW "Skipped: %d" CEST_CLR_RESET, _cest_global_stats.skipped);
        printf("\n");
    }
    
#ifdef CEST_ENABLE_LEAK_DETECTION
    printf("\n" CEST_CLR_BOLD "Memory:" CEST_CLR_RESET "\n");
    if (_cest_alloc_count == _cest_free_count) {
        printf("  " CEST_CLR_GREEN "All %lld allocations freed!" CEST_CLR_RESET "\n", _cest_alloc_count);
    } else {
        long long leaked = _cest_alloc_count - _cest_free_count;
        printf("  " CEST_CLR_RED "Memory leak: %lld allocations not freed" CEST_CLR_RESET "\n", leaked);
    }
#endif
    
    // Write reports
    _cest_write_junit();
    _cest_write_json();
    
    // Post-run command hook.
    // SECURITY: this runs CEST_POST_RUN through the system shell verbatim. Only set
    // this env var to a command you trust — never forward untrusted/external input
    // into it (e.g. from a PR description, webhook payload, etc.).
    const char* post_run = getenv("CEST_POST_RUN");
    if (post_run != NULL) {
        printf("\n" CEST_CLR_BOLD "Running post-test command:" CEST_CLR_RESET " %s\n", post_run);
        int ret = system(post_run);
        if (ret == -1) {
            printf(CEST_CLR_RED "Failed to execute post-run command." CEST_CLR_RESET "\n");
        } else {
#ifdef _WIN32
            printf(CEST_CLR_GREEN "Post-run command executed with exit code %d." CEST_CLR_RESET "\n", ret);
#else
            printf(CEST_CLR_GREEN "Post-run command executed with exit code %d." CEST_CLR_RESET "\n", WEXITSTATUS(ret));
#endif
        }
    }
    
    printf("\n");
    return _cest_global_stats.failed > 0 ? 1 : 0;
}

// ============================================================================
// Optional Namespaced Macros (define CEST_PREFIX before including cest.h)
// ============================================================================
#ifdef CEST_PREFIX
#  define cest_describe(name, block)    describe(name, block)
#  define cest_test(name, block)        test(name, block)
#  define cest_it(name, block)          it(name, block)
#  define cest_expect(x)                expect(x)
#  define cest_expect_array(x, len)     expect_array(x, len)
#  define cest_bench(name, block)       bench(name, block)
#  define cest_beforeEach(fn)           beforeEach(fn)
#  define cest_afterEach(fn)            afterEach(fn)
#  define cest_beforeAll(fn)            beforeAll(fn)
#  define cest_afterAll(fn)             afterAll(fn)
#endif

// ============================================================================
// File-based Test Grouping (describe with automatic filename)
// ============================================================================
#define describe_file(block) \
    describe(_cest_basename(__FILE__), block)

static inline const char* _cest_basename(const char* path) {
    const char* sep = path ? strrchr(path, '/') : NULL;
    return sep ? sep + 1 : (path ? path : "unknown");
}

// ============================================================================
// End Warning Suppression
// ============================================================================
#if CEST_SUPPRESS_WARNINGS
#  ifdef __GNUC__
#    pragma GCC diagnostic pop
#  endif
#  ifdef __clang__
#    pragma clang diagnostic pop
#  endif
#  ifdef _MSC_VER
#    pragma warning(pop)
#  endif
#endif

#endif // CEST_H_INCLUDED
