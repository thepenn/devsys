import Vue from 'vue'
import Router from 'vue-router'

import { getToken, syncTokenFromUrl } from '@/utils/auth'

Vue.use(Router)

const router = new Router({
  mode: 'hash',
  routes: [
    {
      path: '/',
      redirect: '/dashboard'
    },
    {
      path: '/login',
      name: 'Login',
      component: () => import('@/views/login/login.vue'),
      meta: { public: true }
    },
    {
      path: '/dashboard',
      name: 'Dashboard',
      component: () => import('@/views/dashboard/dashboard.vue'),
      meta: { requiresAuth: true }
    },
    {
      path: '/profile',
      name: 'Profile',
      component: () => import('@/views/profile/profile.vue'),
      meta: { requiresAuth: true }
    },
    {
      path: '/admin',
      name: 'Admin',
      component: () => import('@/views/admin/admin.vue'),
      meta: { requiresAuth: true }
    },
    {
      path: '/admin/certificates',
      name: 'AdminCertificates',
      component: () => import('@/views/admin/certificate.vue'),
      meta: { requiresAuth: true }
    },
    {
      path: '/projects/:owner/:name',
      component: () => import('@/views/project/project.vue'),
      meta: { requiresAuth: true },
      children: [
        {
          path: '',
          redirect: 'pipeline'
        },
        {
          path: 'pipeline',
          name: 'ProjectPipeline',
          component: () => import('@/views/project/pipeline.vue'),
          meta: { requiresAuth: true }
        },
        {
          path: 'pipeline/:runId',
          name: 'ProjectPipelineRunDetail',
          component: () => import('@/views/project/build-detail.vue'),
          meta: { requiresAuth: true },
          props: true
        },
        {
          path: 'deployment',
          name: 'ProjectDeployment',
          component: () => import('@/views/project/deployment.vue'),
          meta: { requiresAuth: true }
        },
        {
          path: 'monitor',
          name: 'ProjectMonitor',
          component: () => import('@/views/project/monitor.vue'),
          meta: { requiresAuth: true }
        },
        {
          path: '*',
          redirect: 'pipeline'
        }
      ]
    },
    {
      path: '*',
      redirect: '/login'
    }
  ]
})

router.beforeEach((to, from, next) => {
  const newToken = syncTokenFromUrl()
  const token = newToken || getToken()

  const requiresAuth = to.matched.some(route => route.meta && route.meta.requiresAuth)
  const isPublic = to.matched.some(route => route.meta && route.meta.public)

  if (requiresAuth && !token) {
    next({ path: '/login', query: { error: '请先登录' }})
    return
  }

  if (token && isPublic) {
    next('/dashboard')
    return
  }

  next()
})

export default router
