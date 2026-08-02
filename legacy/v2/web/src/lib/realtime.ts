import { ref } from "vue";

export type RealtimeStatus = "idle" | "connecting" | "connected" | "reconnecting";
export type RealtimeCallback = (event: MessageEvent) => void;

export interface EventStreamLike {
  onopen: ((event: Event) => void) | null;
  onerror: ((event: Event) => void) | null;
  addEventListener(type: string, listener: EventListener): void;
  removeEventListener(type: string, listener: EventListener): void;
  close(): void;
}

export type EventStreamFactory = (url: string) => EventStreamLike;

export class RealtimeHub {
  readonly status = ref<RealtimeStatus>("idle");
  readonly lastEventAt = ref<string | null>(null);

  private readonly callbacks = new Map<string, Map<RealtimeCallback, number>>();
  private readonly eventListener: EventListener;
  private source: EventStreamLike | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private subscriberCount = 0;
  private reconnectAttempt = 0;
  private lastEventId = 0;

  constructor(
    private readonly baseUrl: string,
    private readonly factory: EventStreamFactory = (url) => new EventSource(url),
  ) {
    this.eventListener = (event) => this.handleEvent(event as MessageEvent);
  }

  subscribe(eventTypes: string[], callback: RealtimeCallback): () => void {
    const uniqueTypes = [...new Set(eventTypes)];
    this.subscriberCount += 1;
    for (const eventType of uniqueTypes) {
      const callbacks = this.callbacks.get(eventType) || new Map<RealtimeCallback, number>();
      const firstListener = callbacks.size === 0;
      callbacks.set(callback, (callbacks.get(callback) || 0) + 1);
      this.callbacks.set(eventType, callbacks);
      if (firstListener) this.source?.addEventListener(eventType, this.eventListener);
    }
    if (!this.source && !this.reconnectTimer) this.connect();

    let active = true;
    return () => {
      if (!active) return;
      active = false;
      this.subscriberCount = Math.max(0, this.subscriberCount - 1);
      for (const eventType of uniqueTypes) {
        const callbacks = this.callbacks.get(eventType);
        const references = callbacks?.get(callback) || 0;
        if (references <= 1) callbacks?.delete(callback);
        else callbacks?.set(callback, references - 1);
        if (callbacks?.size === 0) {
          this.callbacks.delete(eventType);
          this.source?.removeEventListener(eventType, this.eventListener);
        }
      }
      if (this.subscriberCount === 0) this.stop(true);
    };
  }

  private connect() {
    if (this.subscriberCount === 0) return;
    this.status.value = this.reconnectAttempt ? "reconnecting" : "connecting";
    const separator = this.baseUrl.includes("?") ? "&" : "?";
    const url = this.lastEventId
      ? `${this.baseUrl}${separator}after=${this.lastEventId}&replay=true`
      : this.baseUrl;

    let source: EventStreamLike;
    try {
      source = this.factory(url);
    } catch {
      this.scheduleReconnect();
      return;
    }
    this.source = source;
    for (const eventType of this.callbacks.keys()) {
      source.addEventListener(eventType, this.eventListener);
    }
    source.onopen = () => {
      if (source !== this.source) return;
      const shouldResync = this.reconnectAttempt > 0;
      this.reconnectAttempt = 0;
      this.status.value = "connected";
      if (shouldResync) this.notifyResync();
    };
    source.onerror = () => {
      if (source !== this.source) return;
      source.close();
      this.source = null;
      this.scheduleReconnect();
    };
  }

  private scheduleReconnect() {
    if (this.subscriberCount === 0 || this.reconnectTimer) return;
    this.status.value = "reconnecting";
    this.reconnectAttempt = Math.min(this.reconnectAttempt + 1, 6);
    const delay = Math.min(30_000, 1_000 * 2 ** (this.reconnectAttempt - 1));
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.connect();
    }, delay);
  }

  private handleEvent(event: MessageEvent) {
    const eventId = Number(event.lastEventId);
    if (Number.isSafeInteger(eventId) && eventId > this.lastEventId) {
      this.lastEventId = eventId;
    }
    this.lastEventAt.value = new Date().toISOString();
    for (const callback of this.callbacks.get(event.type)?.keys() || []) {
      this.callSafely(callback, event);
    }
  }

  private notifyResync() {
    const event = {
      type: "realtime.resynced",
      data: "",
      lastEventId: String(this.lastEventId || ""),
    } as MessageEvent;
    const callbacks = new Set(
      [...this.callbacks.values()].flatMap((items) => [...items.keys()]),
    );
    for (const callback of callbacks) this.callSafely(callback, event);
  }

  private callSafely(callback: RealtimeCallback, event: MessageEvent) {
    try {
      callback(event);
    } catch (error) {
      console.error("Realtime event callback failed", error);
    }
  }

  private stop(resetCursor: boolean) {
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
    this.reconnectTimer = null;
    this.source?.close();
    this.source = null;
    this.reconnectAttempt = 0;
    this.status.value = "idle";
    if (resetCursor) {
      this.lastEventId = 0;
      this.lastEventAt.value = null;
    }
  }
}
