import axios from 'axios'
import { Message } from 'element-ui'
import router from '@/router'
import { getToken, setToken, clearToken } from '@/utils/auth'

// create an axios instance
const service = axios.create({
  baseURL: process.env.VUE_APP_BASE_API || 'http://localhost:8080',
  timeout: 5000 // request timeout
})

// request interceptor
service.interceptors.request.use(
  config => {
    const token = getToken()
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  error => Promise.reject(error)
)

// 响应拦截器
service.interceptors.response.use(
  (response) => {
    // 获取响应头中的 newtoken
    const newToken = response.headers['token']
    if (newToken) {
      setToken(newToken)
    }

    // 返回响应的数据
    return response.data
  },
  error => {
    const { response } = error

    if (response && response.status === 401) {
      clearToken()
      if (router.currentRoute.path !== '/login') {
        router.push('/login')
      }
      return Promise.reject(error)
    }

    // 检查是否在登录页面，如果是则不显示全局错误提示
    // 让各个组件自己处理错误显示
    const isLoginPage = router.currentRoute.path === '/login'

    // 如果不在登录页面，则显示全局错误提示
    if (!isLoginPage) {
      // 其他错误显示消息
      let errorMessage = '请求失败'

      if (response && response.data) {
        if (typeof response.data === 'string') {
          // 纯文本响应
          errorMessage = response.data
        } else if (response.data.message) {
          // JSON格式的message字段
          errorMessage = response.data.message
        } else if (response.data.error) {
          // JSON格式的error字段
          errorMessage = response.data.error
        }
      } else if (error.message) {
        errorMessage = error.message
      }

      Message({
        message: errorMessage,
        type: 'error',
        duration: 5 * 1000
      })
    }

    return Promise.reject(error)
  }
)

// response interceptor
// service.interceptors.response.use(
//   /**
//    * If you want to get http information such as headers or status
//    * Please return  response => response
//   */
//
//   /**
//    * Determine the request status by custom code
//    * Here is just an example
//    * You can also judge the status by HTTP Status Code
//    */
//   response => {
//     const res = response.data
//
//     // if the custom code is not 20000, it is judged as an error.
//     if (res.code !== 20000) {
//       Message({
//         message: res.message || 'Error',
//         type: 'error',
//         duration: 5 * 1000
//       })
//
//       // 50008: Illegal token; 50012: Other clients logged in; 50014: Token expired;
//       if (res.code === 50008 || res.code === 50012 || res.code === 50014) {
//         // to re-login
//         MessageBox.confirm('You have been logged out, you can cancel to stay on this page, or log in again', 'Confirm logout', {
//           confirmButtonText: 'Re-Login',
//           cancelButtonText: 'Cancel',
//           type: 'warning'
//         }).then(() => {
//           store.dispatch('user/resetToken').then(() => {
//             location.reload()
//           })
//         })
//       }
//       return Promise.reject(new Error(res.message || 'Error'))
//     } else {
//       return res
//     }
//   },
//   error => {
//     console.log('err' + error) // for debug
//     Message({
//       message: error.message,
//       type: 'error',
//       duration: 5 * 1000
//     })
//     return Promise.reject(error)
//   }
// )

export default service
