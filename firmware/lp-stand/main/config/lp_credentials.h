// Credentials, read from NVS at boot rather than compiled into the image.
//
// The two secrets - the WiFi key and the broker password - come from NVS and
// from nowhere else. There is no Kconfig option for either, so there is no way
// to compile one in: the image can be built, cached, copied and handed around
// without care, and sdkconfig cannot hold a live secret however it is edited.
//
// What is not secret - the SSID, the broker URI, the account name - keeps a
// Kconfig default, used for any of them that was not provisioned.
//
// Provisioned with `make provision`, from what the EdS tenant issued, or by
// hand with `make wifi-config`. Both write the `creds` partition directly and
// never touch the build.
#pragma once

#include <stdbool.h>

#include "esp_err.h"

// The partition, namespace and key names are the contract between this module
// and both provisioning targets. Changing one side alone does not fail loudly - it
// reads as an unprovisioned stand - so change both together.
#define LP_CRED_PARTITION "creds"
#define LP_CRED_NAMESPACE "lp-stand"

#define LP_CRED_KEY_WIFI_SSID "wifi_ssid"
#define LP_CRED_KEY_WIFI_PASS "wifi_pass"
#define LP_CRED_KEY_MQTT_URI  "mqtt_uri"
#define LP_CRED_KEY_MQTT_USER "mqtt_user"
#define LP_CRED_KEY_MQTT_PASS "mqtt_pass"
#define LP_CRED_KEY_MQTT_CA   "mqtt_ca"

// lp_credentials_load reads the creds partition into memory. It is not an
// error for the partition to be absent or empty: an unprovisioned stand has
// no keys and says so, because a stand that refuses to boot without
// credentials is harder to diagnose than one that boots, lights its strip and
// cannot associate.
//
// Must be called before any accessor below, and before starting WiFi or MQTT.
esp_err_t lp_credentials_load(void);

// True when at least one value came from NVS rather than from Kconfig.
bool lp_credentials_provisioned(void);

const char *lp_cred_wifi_ssid(void);
const char *lp_cred_wifi_password(void);
const char *lp_cred_mqtt_uri(void);
const char *lp_cred_mqtt_username(void);
const char *lp_cred_mqtt_password(void);

// The CA the broker's certificate is verified against, as PEM, or NULL when
// there is none - in which case an mqtts:// connection fails rather than
// proceeding unverified.
//
// Provisioned beside the credentials rather than compiled in, because it is
// the site's and not the firmware's: the substrate has re-rooted its PKI more
// than once, and each time that should cost a reprovision, not a rebuild of
// every stand. The certificate embedded under LP_MQTT_USE_TLS remains as the
// fallback, the way each Kconfig value above does.
const char *lp_cred_mqtt_ca(void);
