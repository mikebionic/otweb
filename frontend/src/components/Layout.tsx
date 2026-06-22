import { Outlet, NavLink } from 'react-router-dom'
import {
  Box, Drawer, List, ListItemButton, ListItemIcon, ListItemText,
  Typography, Divider, AppBar, Toolbar, Chip,
} from '@mui/material'
import {
  Dashboard, Category, Inventory2, Sync, CloudUpload,
  CompareArrows, Label, Settings, Logout, Tune,
} from '@mui/icons-material'

const DRAWER_WIDTH = 230

const navItems = [
  { label: 'Dashboard', icon: <Dashboard />, to: '/dashboard' },
  { label: 'Категории', icon: <Category />, to: '/categories' },
  { label: 'Товары', icon: <Inventory2 />, to: '/products' },
  { label: 'Синхронизация', icon: <Sync />, to: '/sync' },
  { label: 'Push в магазин', icon: <CloudUpload />, to: '/push' },
  { label: 'Mapping', icon: <CompareArrows />, to: '/mapping' },
  { label: 'Атрибуты', icon: <Label />, to: '/attrs' },
  { label: 'Характеристики', icon: <Tune />, to: '/category-features' },
]

const navLinkSx = {
  mx: 1, borderRadius: 2, mb: '2px',
  color: 'rgba(255,255,255,.6)',
  '&.active': { background: 'rgba(99,102,241,.25)', color: '#a5b4fc' },
  '&:hover': { background: 'rgba(255,255,255,.08)', color: '#fff' },
  '& .MuiListItemIcon-root': { minWidth: 36, color: 'inherit' },
}

export default function Layout() {
  const handleLogout = async () => {
    await fetch('/otweb/api/v1/logout', { method: 'POST', credentials: 'include' })
    window.location.href = '/otweb/login'
  }

  return (
    <Box sx={{ display: 'flex', minHeight: '100vh' }}>
      <Drawer
        variant="permanent"
        sx={{
          width: DRAWER_WIDTH,
          flexShrink: 0,
          '& .MuiDrawer-paper': {
            width: DRAWER_WIDTH,
            background: '#1e2130',
            color: '#fff',
            border: 'none',
          },
        }}
      >
        <Box sx={{ p: '20px 18px 16px', borderBottom: '1px solid rgba(255,255,255,.08)' }}>
          <Typography sx={{ fontSize: 10, letterSpacing: '0.12em', color: 'rgba(255,255,255,.4)', textTransform: 'uppercase' }}>
            OTWeb
          </Typography>
          <Typography sx={{ fontSize: 18, fontWeight: 700, color: '#fff', lineHeight: 1.3 }}>
            OTAPI Hub
          </Typography>
        </Box>

        <List sx={{ mt: 1, flexGrow: 1 }}>
          {navItems.map((item) => (
            <ListItemButton key={item.to} component={NavLink} to={item.to} sx={navLinkSx}>
              <ListItemIcon>{item.icon}</ListItemIcon>
              <ListItemText primary={item.label} sx={{ '& .MuiListItemText-primary': { fontSize: 14, fontWeight: 500 } }} />
            </ListItemButton>
          ))}
        </List>

        <Divider sx={{ borderColor: 'rgba(255,255,255,.07)', mx: 1 }} />

        <List>
          <ListItemButton component={NavLink} to="/settings" sx={navLinkSx}>
            <ListItemIcon><Settings /></ListItemIcon>
            <ListItemText primary="Настройки" sx={{ '& .MuiListItemText-primary': { fontSize: 14, fontWeight: 500 } }} />
          </ListItemButton>
          <ListItemButton
            onClick={handleLogout}
            sx={{
              mx: 1, borderRadius: 2, mb: 1,
              color: 'rgba(255,255,255,.4)',
              '&:hover': { background: 'rgba(255,255,255,.06)', color: '#fff' },
              '& .MuiListItemIcon-root': { minWidth: 36, color: 'inherit' },
            }}
          >
            <ListItemIcon><Logout /></ListItemIcon>
            <ListItemText primary="Выход" sx={{ '& .MuiListItemText-primary': { fontSize: 13 } }} />
          </ListItemButton>
        </List>
      </Drawer>

      <Box sx={{ flexGrow: 1, display: 'flex', flexDirection: 'column', minHeight: '100vh' }}>
        <AppBar position="sticky" elevation={0} sx={{ background: '#fff', borderBottom: '1px solid #e9ecef' }}>
          <Toolbar variant="dense" sx={{ justifyContent: 'space-between' }}>
            <Typography sx={{ fontSize: 15, fontWeight: 600, color: '#212529' }}>
              OTAPI Hub
            </Typography>
            <Chip
              label="MySQL · OTAPI Hub"
              size="small"
              variant="outlined"
              sx={{ fontSize: 11, color: '#6c757d', borderColor: '#dee2e6' }}
            />
          </Toolbar>
        </AppBar>

        <Box sx={{ p: 3, flexGrow: 1 }}>
          <Outlet />
        </Box>
      </Box>
    </Box>
  )
}
