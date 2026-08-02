import { createRouter, createWebHistory } from 'vue-router'
import DashboardView from './views/DashboardView.vue'
import MediaView from './views/MediaView.vue'
import ResourceView from './views/ResourceView.vue'
import { navigation } from './navigation'

export const router = createRouter({
  history: createWebHistory(),
  routes: navigation.map((item) => ({
    path: item.path,
    name: item.path,
    component: item.path === '/' || item.path === '/admin' ? DashboardView : item.path === '/media' ? MediaView : ResourceView,
    meta: { item },
  })),
  scrollBehavior: () => ({ top: 0 }),
})
