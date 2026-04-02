# Notifications

Capacitarr provides real-time notifications through Discord webhooks and Apprise (supporting 80+ notification services). Notifications keep you informed about engine activity, disk usage alerts, and system events without needing to check the dashboard. System events are also recorded in the **Activity Log** on the dashboard for at-a-glance visibility.

## Notification Levels

Each notification channel has a **notification level** that controls how much information it receives. Levels are cumulative — higher levels include everything from lower levels plus additional events.

| Level | Value | What it includes |
|-------|-------|------------------|
| **Off** | 0 | Nothing — channel is silenced |
| **Critical** | 1 | Errors, threshold breaches, and integration failures |
| **Important** | 2 | Critical events + mode changes and approval activity |
| **Normal** | 3 | Important events + cycle digests, update notices, and server started *(default)* |
| **Verbose** | 4 | Everything — adds dry-run digests and integration recovery |

### Event Tier Mapping

Every notification event has a fixed tier. A channel receives an event if its level is **≥** the event's tier.

| Event | Tier | Description |
|-------|------|-------------|
| Engine Error | Critical | The evaluation engine encountered an error during a run |
| Threshold Breached | Critical | Disk usage exceeded the configured threshold for a disk group |
| Integration Down | Critical | An integration has failed its connection test |
| Mode Changed | Important | The execution mode was switched (e.g., dry-run → auto) |
| Approval Activity | Important | An item was approved or rejected in the approval queue |
| Cycle Digest | Normal | Summary of each engine run with stats and disk usage |
| Update Available | Normal | A newer Capacitarr release was detected on GitHub |
| Server Started | Normal | Capacitarr has started and is ready to accept requests |
| Dry-Run Digest | Verbose | Cycle digest for dry-run mode engine runs |
| Integration Recovery | Verbose | An integration has recovered from a previous failure |

### Modes and Notification Routing

Execution modes (auto, approval, sunset, dry-run) are **not** separate notification categories. The mode determines the **content** of a digest (title, description, stats) but not whether it gets sent. Routing is determined solely by the channel's notification level and the event's tier:

- **Auto**, **approval**, and **sunset** cycle digests all map to the `cycle_digest` event (tier: Normal)
- **Dry-run** cycle digests map to the `dry_run_digest` event (tier: Verbose)
- **Sunset escalation** fires as a `threshold_breached` alert (tier: Critical) — not a separate sunset event
- **Sunset misconfigured** fires as an `error` alert (tier: Critical) — not a separate sunset event

### Advanced Overrides

For power users, each event type has a per-channel **override** that takes precedence over the tier-based routing. Overrides use a tri-state:

| Override Value | Behavior |
|----------------|----------|
| **Auto** (default) | The notification level determines delivery — no override |
| **On** | Always deliver this event to this channel, regardless of level |
| **Off** | Never deliver this event to this channel, regardless of level |

Overrides are useful when you want a channel at a lower level (e.g., Critical) to still receive a specific Normal-tier event like cycle digests, or when you want to suppress a specific event on a Verbose channel without lowering its level.

## Notification Types

### Cycle Digests

A cycle digest is a single summary notification sent after each engine run completes. Each configured disk group gets its own section in the digest, showing group-specific metrics (items evaluated, candidates, freed space, disk usage). The mode of each disk group determines the content template for its section.

Digest titles vary by execution mode:

| Mode | Title | Summary |
|------|-------|---------|
| **Auto** | 🧹 Cleanup Complete | `Deleted X of Y evaluated items in Z.Zs, freeing N.N GB` |
| **Dry-run** | 🔍 Dry-Run Complete | `Candidates X of Y items in Z.Zs — Would free N.N GB` |
| **Approval** | 📋 Items Queued for Approval | `Queued X of Y items in Z.Zs — Potential N.N GB` |
| *(no action needed)* | ✅ All Clear | `Evaluated X items — no action needed` |

Auto-mode digests also include a disk usage progress bar showing the before/after usage percentage and target. If a newer Capacitarr version is available, a version banner is appended to the digest.

The notification tier determines which channels receive the digest: auto/approval/sunset digests are sent to channels at **Normal** or higher, while dry-run digests are only sent to channels at the **Verbose** level.

### Instant Alerts

Instant alerts fire immediately when their trigger event occurs — they are not batched or delayed. Each alert type covers a specific operational event:

| Alert Type | Tier | Description |
|------------|------|-------------|
| **Engine Error** | Critical | The evaluation engine encountered an error during a run |
| **Mode Changed** | Important | The execution mode was switched (e.g., dry-run → auto) |
| **Server Started** | Normal | Capacitarr has started and is ready to accept requests |
| **Threshold Breached** | Critical | Disk usage has exceeded the configured threshold for a disk group |
| **Update Available** | Normal | A newer Capacitarr release was detected on GitHub |
| **Approval Activity** | Important | An item was approved or rejected in the approval queue |
| **Integration Down** | Critical | An integration has failed its connection test |
| **Integration Recovery** | Verbose | An integration has recovered from a previous failure |

Sunset escalation fires as a **Threshold Breached** alert (when sunset force-expires items to free space). Sunset misconfigured fires as an **Engine Error** alert (when sunset mode is active but no sunset threshold is configured). There is no separate "sunset activity" notification type for routing purposes.

## Discord Setup

### Step 1: Create a Webhook

1. Open your Discord server and navigate to the channel where you want notifications
2. Click the **gear icon** (⚙️) next to the channel name to open Channel Settings
3. Select **Integrations** from the sidebar
4. Click **Webhooks** → **New Webhook**
5. Give the webhook a name (e.g., "Capacitarr") and optionally set an avatar
6. Click **Copy Webhook URL** — you'll need this in the next step
7. Click **Save Changes**

### Step 2: Add Channel in Capacitarr

1. Navigate to **Settings** → **Notifications**
2. Click **Add Channel**
3. Select **Discord** as the channel type
4. Paste the webhook URL you copied from Discord
5. Give the channel a descriptive name (e.g., "Media Alerts")
6. Click **Save**

### Step 3: Configure Notification Level

After saving the channel, set its **notification level** to control which events it receives — see the [Notification Levels](#notification-levels) section above. The default level is **Normal**, which covers cycle digests, update notices, and all critical/important events.

For fine-grained control, expand the **Advanced Overrides** section to force individual event types on or off regardless of the channel's level.

Use the **Test** button to verify the webhook is working. A test notification will appear in your Discord channel.

## Apprise Setup

[Apprise](https://github.com/caronc/apprise) is a self-hosted notification aggregator that supports 80+ notification services including Telegram, Matrix, Pushover, ntfy, Gotify, Email, Slack, Microsoft Teams, and many more. By configuring a single Apprise channel in Capacitarr, you can route notifications to any service Apprise supports.

### Step 1: Deploy an Apprise Server

Run Apprise API as a Docker container alongside Capacitarr:

```yaml
services:
  apprise:
    image: caronc/apprise:latest
    container_name: apprise
    ports:
      - "8000:8000"
    volumes:
      - apprise-config:/config
    restart: unless-stopped

volumes:
  apprise-config:
```

Once running, configure your notification URLs in the Apprise server. Refer to the [Apprise documentation](https://github.com/caronc/apprise/wiki) for supported services and URL formats.

### Step 2: Add Channel in Capacitarr

1. Navigate to **Settings** → **Notifications**
2. Click **Add Channel**
3. Select **Apprise** as the channel type
4. Enter the **Apprise Server URL** — this is the base URL of your Apprise API instance (e.g., `http://apprise:8000`)
5. Optionally enter **Tags** — a comma-separated list of Apprise tags to route the notification to specific destinations (e.g., `telegram,email`). If left empty, all configured notification URLs on the Apprise server receive the message.
6. Give the channel a descriptive name (e.g., "Telegram via Apprise")
7. Click **Save**

### Step 3: Configure Notification Level

After saving the channel, set its **notification level** to control which events it receives — see the [Notification Levels](#notification-levels) section above. The default level is **Normal**.

For fine-grained control, expand the **Advanced Overrides** section to force individual event types on or off regardless of the channel's level.

Use the **Test** button to verify the Apprise connection is working.

### Apprise URL Format

The Apprise Server URL should point to the root of your Apprise API instance. Capacitarr sends notifications to the `POST {url}/api/notify/` endpoint.

**Examples:**

| Network Setup | URL |
|---------------|-----|
| Same Docker network | `http://apprise:8000` |
| Different host | `http://192.168.1.100:8000` |
| Behind reverse proxy | `https://apprise.example.com` |

### Apprise Tags

Tags let you route notifications to specific destinations configured on your Apprise server. For example, if your Apprise server has notification URLs tagged with `urgent` and `info`, you can create two Capacitarr channels — one that sends to `urgent` (for threshold breaches and errors) and one that sends to `info` (for cycle digests).

If no tags are specified, the notification is sent to **all** notification URLs configured on the Apprise server.

## Digest Format

Cycle digest notifications are rendered as rich embeds in Discord and as Markdown messages for Apprise. Each disk group gets its own section in the digest, showing group-specific metrics. Here's what a multi-group auto-mode digest looks like:

```
⚡ Capacitarr v2.0.0 • auto
─────────────────────────────
🧹 Cleanup Complete

Deleted 12 of 97 evaluated items
in 3.2s, freeing 48.3 GB
📦 Included 2 collection group deletion(s)

▓▓▓▓▓▓▓▓▓▓▓▓▓▓░░░░░░ 72% → 65%

📦 v2.1.0 available!
```

Digest components:

- **Author line:** Shows the Capacitarr version and current execution mode
- **Title:** Mode-specific title (🧹 Cleanup Complete, 🔍 Dry-Run Complete, 📋 Items Queued, or ✅ All Clear)
- **Per-group sections:** Each disk group contributes its evaluated items, candidates, deletions, and freed space to the totals
- **Progress bar:** Visual disk usage indicator (auto mode and all-clear only) showing current percentage and target
- **Version banner:** Appears when a newer release is available (optional)

The mode of each disk group determines the content template for its section. For example, a group in auto mode shows deletion counts and freed space, while a group in approval mode shows queued items and potential savings. The notification tier determines **which channels** receive the digest — not which groups appear in it.

Alert notifications use a similar format with a title, message, and color-coded severity (green for success, blue for info, amber for attention, red for errors).
