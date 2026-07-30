import { onBeforeUnmount, onMounted, ref } from "vue";
import { runtime } from "@/lib/runtime";

export function useRealtimeEvents(
  eventTypes: string[],
  onEvent: (event: MessageEvent) => void,
  admin = false,
) {
  const connected = ref(false);
  let source: EventSource | null = null;

  onMounted(() => {
    source = new EventSource(
      `${runtime.apiBase}${admin ? "/admin/events/stream" : "/events/stream"}`,
    );
    source.onopen = () => (connected.value = true);
    source.onerror = () => (connected.value = false);
    for (const eventType of eventTypes) {
      source.addEventListener(eventType, onEvent as EventListener);
    }
  });

  onBeforeUnmount(() => source?.close());
  return { connected };
}
