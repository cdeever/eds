#include "lp_credentials.h"

#include <stdlib.h>
#include <string.h>

#include "esp_log.h"
#include "nvs.h"
#include "nvs_flash.h"
#include "sdkconfig.h"

static const char *TAG = "creds";

// 802.11 caps an SSID at 32 octets and a WPA2 passphrase at 63; the rest are
// sized for what a broker URI and a per-stand account plausibly need. Stored
// as fixed buffers rather than heap because they are read once at boot, live
// for the life of the process, and are handed to esp_mqtt as bare pointers
// that must outlive the call.
#define LP_SSID_CAP      33
#define LP_WIFI_PASS_CAP 64
#define LP_URI_CAP       160
#define LP_USER_CAP      65
#define LP_MQTT_PASS_CAP 65

static char wifi_ssid[LP_SSID_CAP];
static char wifi_pass[LP_WIFI_PASS_CAP];
static char mqtt_uri[LP_URI_CAP];
static char mqtt_user[LP_USER_CAP];
static char mqtt_pass[LP_MQTT_PASS_CAP];

// On the heap, unlike the rest: a PEM root is around 2KB, and a stand that was
// never given one should not carry the buffer. NVS holds a string of at most
// 4000 bytes, which bounds it.
static char *mqtt_ca;

static bool provisioned;

// load_str fills out from NVS, falling back to the given default. An empty
// stored value counts as absent: provisioning a blank is how someone clears a
// field, and it should mean "use the default".
//
// The two keys are loaded with an empty fallback. NVS is the only place they
// can come from, and without one the stand has none.
static bool load_str(nvs_handle_t handle, bool have_handle, const char *key,
                     char *out, size_t cap, const char *fallback)
{
    if (have_handle) {
        size_t len = cap;
        if (nvs_get_str(handle, key, out, &len) == ESP_OK && out[0] != '\0') {
            return true;
        }
    }
    strlcpy(out, fallback, cap);
    return false;
}

// load_ca reads the provisioned CA, if there is one. Sized by asking NVS first
// rather than guessing, and anything that is not a certificate is refused: a
// truncated or mistyped value here fails every handshake with an error that
// does not point back at provisioning.
static bool load_ca(nvs_handle_t handle)
{
    size_t len = 0;
    if (nvs_get_str(handle, LP_CRED_KEY_MQTT_CA, NULL, &len) != ESP_OK || len < 2) {
        return false;
    }

    char *pem = malloc(len);
    if (pem == NULL) {
        ESP_LOGW(TAG, "no memory for a %u byte CA", (unsigned)len);
        return false;
    }
    if (nvs_get_str(handle, LP_CRED_KEY_MQTT_CA, pem, &len) != ESP_OK ||
        strstr(pem, "-----BEGIN CERTIFICATE-----") == NULL) {
        ESP_LOGW(TAG, "%s is not a PEM certificate; ignoring it", LP_CRED_KEY_MQTT_CA);
        free(pem);
        return false;
    }

    mqtt_ca = pem;
    return true;
}

esp_err_t lp_credentials_load(void)
{
    nvs_handle_t handle = 0;
    bool have_handle = false;

    esp_err_t err = nvs_flash_init_partition(LP_CRED_PARTITION);
    if (err == ESP_ERR_NVS_NO_FREE_PAGES || err == ESP_ERR_NVS_NEW_VERSION_FOUND) {
        // A partition written by an older tool, or half-written. Erasing loses
        // only credentials, which can be reprovisioned in seconds.
        ESP_LOGW(TAG, "%s unreadable (%s); erasing", LP_CRED_PARTITION, esp_err_to_name(err));
        if (nvs_flash_erase_partition(LP_CRED_PARTITION) == ESP_OK) {
            err = nvs_flash_init_partition(LP_CRED_PARTITION);
        }
    }

    if (err == ESP_OK) {
        err = nvs_open_from_partition(LP_CRED_PARTITION, LP_CRED_NAMESPACE, NVS_READONLY, &handle);
        have_handle = (err == ESP_OK);
        if (!have_handle && err != ESP_ERR_NVS_NOT_FOUND) {
            ESP_LOGW(TAG, "opening %s/%s: %s", LP_CRED_PARTITION, LP_CRED_NAMESPACE,
                     esp_err_to_name(err));
        }
    } else {
        ESP_LOGW(TAG, "no usable %s partition: %s", LP_CRED_PARTITION, esp_err_to_name(err));
    }

    bool from_nvs = false;
    from_nvs |= load_str(handle, have_handle, LP_CRED_KEY_WIFI_SSID,
                         wifi_ssid, sizeof(wifi_ssid), CONFIG_LP_WIFI_SSID);
    from_nvs |= load_str(handle, have_handle, LP_CRED_KEY_WIFI_PASS,
                         wifi_pass, sizeof(wifi_pass), "");
    from_nvs |= load_str(handle, have_handle, LP_CRED_KEY_MQTT_URI,
                         mqtt_uri, sizeof(mqtt_uri), CONFIG_LP_MQTT_URI);
    from_nvs |= load_str(handle, have_handle, LP_CRED_KEY_MQTT_USER,
                         mqtt_user, sizeof(mqtt_user), CONFIG_LP_MQTT_USERNAME);
    from_nvs |= load_str(handle, have_handle, LP_CRED_KEY_MQTT_PASS,
                         mqtt_pass, sizeof(mqtt_pass), "");
    bool ca_from_nvs = have_handle && load_ca(handle);
    from_nvs |= ca_from_nvs;
    provisioned = from_nvs;

#if CONFIG_LP_MQTT_USE_TLS
    if (mqtt_ca == NULL) {
        extern const char mqtt_ca_pem_start[] asm("_binary_mqtt_ca_pem_start");
        mqtt_ca = (char *)mqtt_ca_pem_start;
    }
#endif

    if (have_handle) {
        nvs_close(handle);
    }

    // Deliberately never logs a password, only whether one is present: the
    // console is the first place a secret leaks, and "is it set" is the whole
    // question when a stand will not associate.
    ESP_LOGI(TAG, "%s; ssid '%s', wifi key %s, mqtt user '%s', mqtt key %s, broker CA %s",
             provisioned ? "provisioned from " LP_CRED_PARTITION : "not provisioned",
             wifi_ssid, wifi_pass[0] ? "set" : "empty",
             mqtt_user, mqtt_pass[0] ? "set" : "empty",
             ca_from_nvs ? "provisioned" : mqtt_ca != NULL ? "compiled in" : "absent");

    return ESP_OK;
}

bool lp_credentials_provisioned(void)
{
    return provisioned;
}

const char *lp_cred_wifi_ssid(void)     { return wifi_ssid; }
const char *lp_cred_wifi_password(void) { return wifi_pass; }
const char *lp_cred_mqtt_uri(void)      { return mqtt_uri; }
const char *lp_cred_mqtt_username(void) { return mqtt_user; }
const char *lp_cred_mqtt_password(void) { return mqtt_pass; }
const char *lp_cred_mqtt_ca(void)       { return mqtt_ca; }
