#include "status.h"

lp_fault_t lp_fault_of(const lp_link_t *link)
{
    if (link == NULL || link->broker) {
        return LP_FAULT_NONE;
    }
    if (!link->provisioned) {
        return LP_FAULT_UNPROVISIONED;
    }
    if (!link->wifi) {
        return LP_FAULT_NO_WIFI;
    }
    return link->refused ? LP_FAULT_REFUSED : LP_FAULT_NO_BROKER;
}

lp_fault_t lp_status_step(lp_status_t *status, lp_fault_t fault, uint32_t t_ms)
{
    if (status == NULL) {
        return LP_FAULT_NONE;
    }
    if (fault == LP_FAULT_NONE) {
        status->troubled = false;
        return LP_FAULT_NONE;
    }
    if (!status->troubled) {
        status->troubled = true;
        status->since_ms = t_ms;
    }
    // Unsigned, so this is right across the timestamp wrapping at 49 days.
    return t_ms - status->since_ms >= LP_FAULT_GRACE_MS ? fault : LP_FAULT_NONE;
}

bool lp_status_lit(lp_fault_t fault, uint32_t t_ms)
{
    if (fault <= LP_FAULT_NONE) {
        return false;
    }
    const uint32_t blink = LP_FAULT_BLINK_ON_MS + LP_FAULT_BLINK_OFF_MS;
    const uint32_t cycle = (uint32_t)fault * blink + LP_FAULT_PAUSE_MS;
    const uint32_t at = t_ms % cycle;
    return at < (uint32_t)fault * blink && at % blink < LP_FAULT_BLINK_ON_MS;
}

void lp_status_overlay(lp_fault_t fault, uint32_t t_ms, lp_rgb_t *frame, size_t count)
{
    if (fault <= LP_FAULT_NONE || frame == NULL || count == 0) {
        return;
    }
    frame[0] = lp_status_lit(fault, t_ms) ? (lp_rgb_t){.r = 255, .g = 0, .b = 0} : (lp_rgb_t){0};
}
