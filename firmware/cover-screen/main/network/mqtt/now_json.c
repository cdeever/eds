#include "now_json.h"

#include <string.h>

#include "cJSON.h"

bool cs_now_parse(const char *json, size_t len, cs_now_t *now)
{
    cJSON *root = cJSON_ParseWithLength(json, len);
    if (root == NULL) {
        return false;
    }

    bool ok = false;
    const cJSON *v = cJSON_GetObjectItemCaseSensitive(root, "v");
    const cJSON *state = cJSON_GetObjectItemCaseSensitive(root, "state");

    if (cJSON_IsNumber(v) && v->valueint == CS_NOWPLAYING_VERSION && cJSON_IsString(state)) {
        ok = true;
        if (strcmp(state->valuestring, "playing") == 0) {
            now->play = CS_PLAY_PLAYING;
        } else if (strcmp(state->valuestring, "paused") == 0) {
            now->play = CS_PLAY_PAUSED;
        } else if (strcmp(state->valuestring, "stopped") == 0) {
            now->play = CS_PLAY_STOPPED;
        } else {
            ok = false;
        }

        // A track with no cover of its own carries no "art", and the cover
        // topic then still holds an earlier track's (ADR-0016). This is what
        // keeps the screen from showing it.
        const cJSON *art = cJSON_GetObjectItemCaseSensitive(root, "art");
        const cJSON *hash = cJSON_IsObject(art) ? cJSON_GetObjectItemCaseSensitive(art, "sha256") : NULL;
        now->has_art = cJSON_IsString(hash) && hash->valuestring[0] != '\0';

        // Only the word itself, and only "true": anything else, or nothing,
        // leaves the buttons where they were.
        now->no_controls = cJSON_IsTrue(cJSON_GetObjectItemCaseSensitive(root, "no_controls"));
    }

    cJSON_Delete(root);
    return ok;
}
