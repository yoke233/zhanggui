import { create } from "zustand";
import type { Notification } from "../types/apiV2";
import type { ApiClient } from "../lib/apiClient";
import type { WsClient } from "../lib/wsClient";
import type { NotificationEventPayload } from "../types/ws";
import { BrowserNotificationService } from "../lib/browserNotifications";

interface NotificationState {
  notifications: Notification[];
  unreadCount: number;
  loading: boolean;
  browserService: BrowserNotificationService;

  // Actions
  init: (api: ApiClient, ws: WsClient) => () => void;
  fetchNotifications: (api: ApiClient) => Promise<void>;
  fetchUnreadCount: (api: ApiClient) => Promise<void>;
  markRead: (api: ApiClient, id: number) => Promise<void>;
  markAllRead: (api: ApiClient) => Promise<void>;
  deleteNotification: (api: ApiClient, id: number) => Promise<void>;
  requestBrowserPermission: () => Promise<NotificationPermission>;
}

export const useNotificationStore = create<NotificationState>((set, get) => ({
  notifications: [],
  unreadCount: 0,
  loading: false,
  browserService: new BrowserNotificationService(),

  init: (api, ws) => {
    // Load initial data.
    get().fetchNotifications(api);
    get().fetchUnreadCount(api);

    // Subscribe to real-time notification events via WebSocket.
    const unsubCreated = ws.subscribe<NotificationEventPayload>(
      "notification.created",
      (payload) => {
        if (payload?.notification) {
          const n = payload.notification as unknown as Notification;
          set((state) => ({
            notifications: [n, ...state.notifications],
            unreadCount: state.unreadCount + 1,
          }));

          // Fire browser notification if channel includes "browser".
          if (
            !n.channels ||
            n.channels.includes("browser") ||
            n.channels.includes("in_app")
          ) {
            get().browserService.show(n.title, {
              body: n.body,
              tag: `notification-${n.id}`,
              data: { actionUrl: n.action_url },
            });
          }
        }
      },
    );

    const unsubRead = ws.subscribe<NotificationEventPayload>(
      "notification.read",
      (payload) => {
        if (payload?.notification_id) {
          const id = payload.notification_id;
          set((state) => ({
            notifications: state.notifications.map((n) =>
              n.id === id ? { ...n, read: true, read_at: new Date().toISOString() } : n,
            ),
            unreadCount: Math.max(0, state.unreadCount - 1),
          }));
        }
      },
    );

    const unsubAllRead = ws.subscribe(
      "notification.all_read",
      () => {
        set((state) => ({
          notifications: state.notifications.map((n) => ({
            ...n,
            read: true,
            read_at: n.read_at ?? new Date().toISOString(),
          })),
          unreadCount: 0,
        }));
      },
    );

    // Return cleanup function.
    return () => {
      unsubCreated();
      unsubRead();
      unsubAllRead();
    };
  },

  fetchNotifications: async (api) => {
    set({ loading: true });
    try {
      const notifications = await api.listNotifications({ limit: 50 });
      set({ notifications, loading: false });
    } catch {
      set({ loading: false });
    }
  },

  fetchUnreadCount: async (api) => {
    try {
      const { count } = await api.getUnreadNotificationCount();
      set({ unreadCount: count });
    } catch {
      // ignore
    }
  },

  markRead: async (api, id) => {
    try {
      await api.markNotificationRead(id);
      set((state) => ({
        notifications: state.notifications.map((n) =>
          n.id === id ? { ...n, read: true, read_at: new Date().toISOString() } : n,
        ),
        unreadCount: Math.max(0, state.unreadCount - 1),
      }));
    } catch {
      // ignore
    }
  },

  markAllRead: async (api) => {
    try {
      await api.markAllNotificationsRead();
      set((state) => ({
        notifications: state.notifications.map((n) => ({
          ...n,
          read: true,
          read_at: n.read_at ?? new Date().toISOString(),
        })),
        unreadCount: 0,
      }));
    } catch {
      // ignore
    }
  },

  deleteNotification: async (api, id) => {
    try {
      await api.deleteNotification(id);
      set((state) => ({
        notifications: state.notifications.filter((n) => n.id !== id),
        unreadCount: state.notifications.find((n) => n.id === id && !n.read)
          ? Math.max(0, state.unreadCount - 1)
          : state.unreadCount,
      }));
    } catch {
      // ignore
    }
  },

  requestBrowserPermission: async () => {
    return get().browserService.requestPermission();
  },
}));
