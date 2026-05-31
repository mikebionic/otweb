import axios from 'axios'

const api = axios.create({
  baseURL: '/otweb/api/v1',
  withCredentials: true,
})

api.interceptors.response.use(
  (res) => res,
  (err) => {
    if (err.response?.status === 401) {
      window.location.href = '/otweb/login'
    }
    return Promise.reject(err)
  }
)

export default api
