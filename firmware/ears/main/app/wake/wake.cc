#include "wake.h"

#include <cstring>

#include "esp_heap_caps.h"
#include "esp_log.h"
#include "frontend.h"
#include "frontend_util.h"
#include "sdkconfig.h"
#include "tensorflow/lite/micro/micro_allocator.h"
#include "tensorflow/lite/micro/micro_interpreter.h"
#include "tensorflow/lite/micro/micro_mutable_op_resolver.h"
#include "tensorflow/lite/micro/micro_resource_variable.h"
#include "tensorflow/lite/schema/schema_generated.h"

static const char *TAG = "wake";

extern const uint8_t model_start[] asm("_binary_hey_jarvis_tflite_start");

// The frontend, as ESPHome sets it over TensorFlow's defaults
// (esphome/components/micro_wake_word/preprocessor_settings.h): the model
// knows no other features.
static constexpr int FEATURES = 40;
static constexpr int FEATURE_MS = 30;

// After a reset, this many features below the cutoff before a detection can
// count: a second, as ESPHome waits.
static constexpr int COOL_OFF = 100;

// The model's state variables live apart from its tensors, in an arena of
// their own; ESPHome's size.
static constexpr size_t VARIABLE_ARENA = 1024;

static FrontendConfig frontend_config;
static FrontendState frontend;
static tflite::MicroInterpreter *interpreter;
static TfLiteTensor *input;
static int stride;      // features the model takes at a time
static int stride_step; // of them, how many it has

static uint8_t window[CONFIG_EA_WAKE_WINDOW];
static int window_at;
static int cool_off = -COOL_OFF;
static uint8_t peak;

static bool register_ops(tflite::MicroMutableOpResolver<20> &ops)
{
    // The operations a microWakeWord streaming model is made of, as ESPHome
    // registers them.
    return ops.AddCallOnce() == kTfLiteOk && ops.AddVarHandle() == kTfLiteOk && ops.AddReshape() == kTfLiteOk &&
           ops.AddReadVariable() == kTfLiteOk && ops.AddStridedSlice() == kTfLiteOk &&
           ops.AddConcatenation() == kTfLiteOk && ops.AddAssignVariable() == kTfLiteOk &&
           ops.AddConv2D() == kTfLiteOk && ops.AddMul() == kTfLiteOk && ops.AddAdd() == kTfLiteOk &&
           ops.AddMean() == kTfLiteOk && ops.AddFullyConnected() == kTfLiteOk && ops.AddLogistic() == kTfLiteOk &&
           ops.AddQuantize() == kTfLiteOk && ops.AddDepthwiseConv2D() == kTfLiteOk &&
           ops.AddAveragePool2D() == kTfLiteOk && ops.AddMaxPool2D() == kTfLiteOk && ops.AddPad() == kTfLiteOk &&
           ops.AddPack() == kTfLiteOk && ops.AddSplitV() == kTfLiteOk;
}

extern "C" esp_err_t ea_wake_init(void)
{
    FrontendFillConfigWithDefaults(&frontend_config);
    frontend_config.window.size_ms = FEATURE_MS;
    frontend_config.window.step_size_ms = CONFIG_EA_WAKE_STEP_MS;
    frontend_config.filterbank.num_channels = FEATURES;
    frontend_config.filterbank.lower_band_limit = 125.0f;
    frontend_config.filterbank.upper_band_limit = 7500.0f;
    frontend_config.noise_reduction.smoothing_bits = 10;
    frontend_config.noise_reduction.even_smoothing = 0.025f;
    frontend_config.noise_reduction.odd_smoothing = 0.06f;
    frontend_config.noise_reduction.min_signal_remaining = 0.05f;
    frontend_config.pcan_gain_control.enable_pcan = 1;
    frontend_config.pcan_gain_control.strength = 0.95f;
    frontend_config.pcan_gain_control.offset = 80.0f;
    frontend_config.pcan_gain_control.gain_bits = 21;
    frontend_config.log_scale.enable_log = 1;
    frontend_config.log_scale.scale_shift = 6;
    if (!FrontendPopulateState(&frontend_config, &frontend, 16000)) {
        ESP_LOGE(TAG, "the audio frontend could not be set up");
        return ESP_ERR_NO_MEM;
    }

    const tflite::Model *model = tflite::GetModel(model_start);
    if (model->version() != TFLITE_SCHEMA_VERSION) {
        ESP_LOGE(TAG, "the model is schema %lu, this build reads %d", (unsigned long)model->version(),
                 TFLITE_SCHEMA_VERSION);
        return ESP_ERR_INVALID_VERSION;
    }

    static tflite::MicroMutableOpResolver<20> ops;
    if (!register_ops(ops)) {
        return ESP_FAIL;
    }

    // The manifest's arena size is ESPHome's measure on its build; this one
    // may need more, and ESPHome tries up to twice it. Internal RAM: every
    // invocation reads all of it, thirty times a second.
    size_t arena_size = 2 * (size_t)CONFIG_EA_WAKE_ARENA;
    uint8_t *arena = (uint8_t *)heap_caps_aligned_alloc(16, arena_size, MALLOC_CAP_INTERNAL | MALLOC_CAP_8BIT);
    uint8_t *variables = (uint8_t *)heap_caps_aligned_alloc(16, VARIABLE_ARENA, MALLOC_CAP_INTERNAL | MALLOC_CAP_8BIT);
    if (arena == nullptr || variables == nullptr) {
        ESP_LOGE(TAG, "no memory for the model's %u-byte arena", (unsigned)arena_size);
        return ESP_ERR_NO_MEM;
    }
    tflite::MicroAllocator *variable_allocator = tflite::MicroAllocator::Create(variables, VARIABLE_ARENA);
    tflite::MicroResourceVariables *resources = tflite::MicroResourceVariables::Create(variable_allocator, 20);

    interpreter = new tflite::MicroInterpreter(model, ops, arena, arena_size, resources);
    if (interpreter->AllocateTensors() != kTfLiteOk) {
        ESP_LOGE(TAG, "the model's tensors do not fit in %u bytes", (unsigned)arena_size);
        return ESP_ERR_NO_MEM;
    }
    input = interpreter->input(0);
    TfLiteTensor *output = interpreter->output(0);
    if (input->dims->size != 3 || input->dims->data[0] != 1 || input->dims->data[2] != FEATURES ||
        input->type != kTfLiteInt8 || output->type != kTfLiteUInt8) {
        ESP_LOGE(TAG, "the model does not take what microWakeWord's do");
        return ESP_ERR_INVALID_ARG;
    }
    stride = input->dims->data[1];
    ESP_LOGI(TAG, "\"%s\": %d features at a time, %u of %u arena bytes used; wakes above %d/255 over %d",
             CONFIG_EA_WAKE_WORD, stride, (unsigned)interpreter->arena_used_bytes(), (unsigned)arena_size,
             CONFIG_EA_WAKE_CUTOFF, CONFIG_EA_WAKE_WINDOW);
    return ESP_OK;
}

// One feature into the model; true when the window's mean passes the cutoff.
static bool hear_feature(const int8_t *feature)
{
    std::memcpy(input->data.int8 + FEATURES * stride_step, feature, FEATURES);
    if (++stride_step < stride) {
        return false;
    }
    stride_step = 0;
    if (interpreter->Invoke() != kTfLiteOk) {
        ESP_LOGW(TAG, "the model could not be run");
        return false;
    }
    window_at = (window_at + 1) % CONFIG_EA_WAKE_WINDOW;
    window[window_at] = interpreter->output(0)->data.uint8[0];

    if (window[window_at] < CONFIG_EA_WAKE_CUTOFF && cool_off < 0) {
        cool_off++;
    }
    uint32_t sum = 0;
    for (int i = 0; i < CONFIG_EA_WAKE_WINDOW; i++) {
        sum += window[i];
    }
    uint8_t mean = (uint8_t)(sum / CONFIG_EA_WAKE_WINDOW);
    if (mean > peak) {
        peak = mean;
    }
    return cool_off >= 0 && sum > (uint32_t)CONFIG_EA_WAKE_CUTOFF * CONFIG_EA_WAKE_WINDOW;
}

extern "C" bool ea_wake_feed(const int16_t *samples, size_t n)
{
    if (interpreter == nullptr) {
        return false;
    }
    bool woke = false;
    while (n > 0) {
        size_t read = 0;
        FrontendOutput out = FrontendProcessSamples(&frontend, samples, n, &read);
        samples += read;
        n -= read;
        if (out.values == nullptr || out.size != FEATURES) {
            if (read == 0) {
                break; // nothing taken, nothing made: never spin
            }
            continue; // not a whole step yet
        }
        // As ESPHome scales them: the frontend's 0 to 670 or so, as the
        // model's int8, (v * 256) / (25.6 * 26.0) - 128.
        int8_t feature[FEATURES];
        for (int i = 0; i < FEATURES; i++) {
            int32_t v = ((int32_t)out.values[i] * 256 + 333) / 666 - 128;
            feature[i] = (int8_t)(v < -128 ? -128 : v > 127 ? 127 : v);
        }
        woke |= hear_feature(feature);
        if (read == 0) {
            break;
        }
    }
    return woke;
}

// As ESPHome resets after a detection: the window emptied and the cool-off
// begun. The model's own state and the frontend's run on; they are the
// recent past, which the next detection needs.
extern "C" void ea_wake_reset(void)
{
    std::memset(window, 0, sizeof(window));
    cool_off = -COOL_OFF;
}

extern "C" uint8_t ea_wake_peak(void)
{
    uint8_t p = peak;
    peak = 0;
    return p;
}
