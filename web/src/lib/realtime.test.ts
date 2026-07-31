import { afterEach, describe, expect, it, vi } from "vitest";
import {
  RealtimeHub,
  type EventStreamFactory,
  type EventStreamLike,
} from "./realtime";

class FakeEventStream implements EventStreamLike {
  onopen: ((event: Event) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  readonly listeners = new Map<string, Set<EventListener>>();
  closed = false;

  constructor(readonly url: string) {}

  addEventListener(type: string, listener: EventListener) {
    const listeners = this.listeners.get(type) || new Set<EventListener>();
    listeners.add(listener);
    this.listeners.set(type, listeners);
  }

  removeEventListener(type: string, listener: EventListener) {
    this.listeners.get(type)?.delete(listener);
  }

  close() {
    this.closed = true;
  }

  open() {
    this.onopen?.({ type: "open" } as Event);
  }

  fail() {
    this.onerror?.({ type: "error" } as Event);
  }

  emit(type: string, id: string) {
    const event = { type, data: "{}", lastEventId: id } as MessageEvent;
    for (const listener of this.listeners.get(type) || []) listener(event);
  }
}

afterEach(() => {
  vi.useRealTimers();
});

describe("RealtimeHub", () => {
  it("reuses one event stream for all page subscribers", () => {
    const streams: FakeEventStream[] = [];
    const factory: EventStreamFactory = (url) => {
      const stream = new FakeEventStream(url);
      streams.push(stream);
      return stream;
    };
    const hub = new RealtimeHub("/api/v1/events/stream", factory);
    const first = vi.fn();
    const second = vi.fn();

    const unsubscribeFirst = hub.subscribe(["review.updated"], first);
    const unsubscribeSecond = hub.subscribe(["notification.created"], second);
    expect(streams).toHaveLength(1);

    streams[0].open();
    streams[0].emit("review.updated", "12");
    expect(first).toHaveBeenCalledTimes(1);
    expect(second).not.toHaveBeenCalled();
    expect(hub.status.value).toBe("connected");

    unsubscribeFirst();
    expect(streams[0].closed).toBe(false);
    unsubscribeSecond();
    expect(streams[0].closed).toBe(true);
    expect(hub.status.value).toBe("idle");
  });

  it("reconnects with the last event cursor and requests a full resync", () => {
    vi.useFakeTimers();
    const streams: FakeEventStream[] = [];
    const factory: EventStreamFactory = (url) => {
      const stream = new FakeEventStream(url);
      streams.push(stream);
      return stream;
    };
    const hub = new RealtimeHub("/api/v1/admin/events/stream", factory);
    const callback = vi.fn();

    const unsubscribe = hub.subscribe(["ticket.updated"], callback);
    streams[0].open();
    streams[0].emit("ticket.updated", "42");
    callback.mockClear();
    streams[0].fail();

    expect(hub.status.value).toBe("reconnecting");
    vi.advanceTimersByTime(1_000);
    expect(streams).toHaveLength(2);
    expect(streams[1].url).toContain("after=42");
    expect(streams[1].url).toContain("replay=true");

    streams[1].open();
    expect(callback).toHaveBeenCalledTimes(1);
    expect(callback.mock.calls[0][0].type).toBe("realtime.resynced");
    expect(hub.status.value).toBe("connected");
    unsubscribe();
  });
});
