import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { router } from './router'
import './styles.css'
import './inspector.css'
import './result.css'

createApp(App).use(createPinia()).use(router).mount('#app')
