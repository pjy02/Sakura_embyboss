import { computed, onBeforeUnmount, onMounted } from "vue";
import { RealtimeHub } from "@/lib/realtime";
import { runtime } from "@/lib/runtime";

const hubs = new Map<"admin" | "portal", RealtimeHub>();

function getHub(admin: boolean) {
  const area = admin ? "admin" : "portal";
  let hub = hubs.get(area);
  if (!hub) {
    hub = new RealtimeHub(
      `${runtime.apiBase}${admin ? "/admin/events/stream" : "/events/stream"}`,
    );
    hubs.set(area, hub);
  }
  return hub;
}

export function useRealtimeEvents(
  eventTypes: string[],
  onEvent: (event: MessageEvent) => void,
  admin = false,
) {
  const hub = getHub(admin);
  const connected = computed(() => hub.status.value === "connected");
  let unsubscribe: (() => void) | null = null;

  onMounted(() => {
    unsubscribe = hub.subscribe(eventTypes, onEvent);
  });

  onBeforeUnmount(() => unsubscribe?.());
  return { connected, status: hub.status, lastEventAt: hub.lastEventAt };
}
