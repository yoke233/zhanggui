/**
 * BrowserNotificationService wraps the Web Notification API with
 * cross-platform support for desktop and mobile (PWA).
 *
 * Desktop browsers: Uses the standard Notification API directly.
 * Mobile browsers (PWA): Uses ServiceWorker registration.showNotification()
 *   which is required for notifications to work on mobile platforms (iOS/Android).
 *
 * Usage:
 *   const svc = new BrowserNotificationService();
 *   await svc.requestPermission();
 *   svc.show("Task completed", { body: "Issue #42 finished" });
 */

export interface BrowserNotificationOptions {
  body?: string;
  icon?: string;
  badge?: string;
  tag?: string;
  /** If true, the notification persists until the user interacts with it. */
  requireInteraction?: boolean;
  /** Vibration pattern for mobile devices (ms array). */
  vibrate?: number[];
  /** Arbitrary data attached to the notification (e.g. deep-link URL). */
  data?: Record<string, unknown>;
  /** Auto-close timeout in milliseconds. Default: 5000 (desktop only). */
  autoCloseMs?: number;
}

export class BrowserNotificationService {
  private swRegistration: ServiceWorkerRegistration | null = null;

  /** Whether the Notification API is available in this environment. */
  get isSupported(): boolean {
    return typeof window !== "undefined" && "Notification" in window;
  }

  /** Whether we're running as a PWA (service worker available). */
  get isPWA(): boolean {
    return (
      typeof navigator !== "undefined" &&
      "serviceWorker" in navigator &&
      (window.matchMedia("(display-mode: standalone)").matches ||
        (navigator as unknown as { standalone?: boolean }).standalone === true)
    );
  }

  /** Current permission state: "granted", "denied", or "default". */
  get permission(): NotificationPermission {
    if (!this.isSupported) return "denied";
    return Notification.permission;
  }

  /**
   * Request notification permission from the user.
   * Returns the resulting permission state.
   */
  async requestPermission(): Promise<NotificationPermission> {
    if (!this.isSupported) return "denied";
    if (Notification.permission === "granted") return "granted";
    if (Notification.permission === "denied") return "denied";

    try {
      const result = await Notification.requestPermission();
      // If granted and we have a service worker, cache the registration.
      if (result === "granted") {
        await this.ensureServiceWorkerRegistration();
      }
      return result;
    } catch {
      return "denied";
    }
  }

  /**
   * Show a notification. Uses ServiceWorker on mobile, direct API on desktop.
   *
   * @param title - Notification title
   * @param options - Additional notification options
   */
  show(title: string, options?: BrowserNotificationOptions): void {
    if (!this.isSupported || Notification.permission !== "granted") return;

    const notifOptions: NotificationOptions = {
      body: options?.body,
      icon: options?.icon ?? this.defaultIcon(),
      badge: options?.badge,
      tag: options?.tag,
      requireInteraction: options?.requireInteraction ?? false,
      vibrate: options?.vibrate ?? [200, 100, 200],
      data: options?.data,
    };

    // Mobile / PWA path: use ServiceWorker showNotification for reliability.
    if (this.usesServiceWorker()) {
      this.showViaServiceWorker(title, notifOptions);
      return;
    }

    // Desktop path: use direct Notification API.
    this.showDirect(title, notifOptions, options?.autoCloseMs ?? 5000);
  }

  /**
   * Close all notifications with the given tag.
   */
  async closeByTag(tag: string): Promise<void> {
    if (!this.swRegistration) return;
    try {
      const notifications = await this.swRegistration.getNotifications({ tag });
      notifications.forEach((n) => n.close());
    } catch {
      // ignore
    }
  }

  // ── Private ──

  private usesServiceWorker(): boolean {
    return typeof navigator !== "undefined" && "serviceWorker" in navigator;
  }

  private async ensureServiceWorkerRegistration(): Promise<ServiceWorkerRegistration | null> {
    if (this.swRegistration) return this.swRegistration;
    if (!("serviceWorker" in navigator)) return null;

    try {
      this.swRegistration = await navigator.serviceWorker.ready;
      return this.swRegistration;
    } catch {
      return null;
    }
  }

  private async showViaServiceWorker(title: string, options: NotificationOptions): Promise<void> {
    const reg = await this.ensureServiceWorkerRegistration();
    if (reg) {
      try {
        await reg.showNotification(title, options);
        return;
      } catch {
        // Fallback to direct API if showNotification fails.
      }
    }
    // Fallback: direct Notification API.
    this.showDirect(title, options, 5000);
  }

  private showDirect(title: string, options: NotificationOptions, autoCloseMs: number): void {
    try {
      const notification = new Notification(title, options);

      notification.onclick = (event) => {
        event.preventDefault();
        window.focus();
        const actionUrl = (options.data as Record<string, unknown> | undefined)?.actionUrl;
        if (typeof actionUrl === "string" && actionUrl) {
          window.location.href = actionUrl;
        }
        notification.close();
      };

      if (autoCloseMs > 0) {
        setTimeout(() => notification.close(), autoCloseMs);
      }
    } catch {
      // Some browsers (e.g. Android Chrome without SW) throw on direct `new Notification()`.
      // In that case, just silently fail - in-app notification is still shown.
    }
  }

  private defaultIcon(): string {
    // Use the app's favicon if available.
    const link = typeof document !== "undefined"
      ? document.querySelector<HTMLLinkElement>("link[rel~='icon']")
      : null;
    return link?.href ?? "/favicon.ico";
  }
}
