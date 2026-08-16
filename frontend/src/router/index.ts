import { createRouter, createWebHashHistory } from 'vue-router'
import HashView from '../views/hash-view.vue'
import VerifyView from '../views/verify-view.vue'
import CompareView from '../views/compare-view.vue'
import BatchView from '../views/batch-view.vue'

// Wails 桌面环境无服务端路由，使用 hash 历史模式。
export const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', name: 'hash', component: HashView },
    { path: '/verify', name: 'verify', component: VerifyView },
    { path: '/compare', name: 'compare', component: CompareView },
    { path: '/batch', name: 'batch', component: BatchView },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})
