import { Admin, Resource, CustomRoutes } from 'react-admin';
import { Route } from 'react-router-dom';
import DnsIcon from '@mui/icons-material/Dns';
import ViewListIcon from '@mui/icons-material/ViewList';
import LayersIcon from '@mui/icons-material/Layers';
import DeviceHubIcon from '@mui/icons-material/DeviceHub';
import StorageIcon from '@mui/icons-material/Storage';
import LockIcon from '@mui/icons-material/Lock';
import SettingsIcon from '@mui/icons-material/Settings';
import AssignmentIcon from '@mui/icons-material/Assignment';
import HistoryIcon from '@mui/icons-material/History';
import NetworkCheckIcon from '@mui/icons-material/NetworkCheck';

import dataProvider from './dataProvider';
import authProvider from './authProvider';
import Dashboard from './Dashboard';
import { ServiceList, ServiceShow, NodeList, NodeShow, StackList, NetworkList, VolumeList, SecretList, ConfigList, TaskList, AuditList } from './resources';

const App = () => (
  <Admin dashboard={Dashboard} dataProvider={dataProvider} authProvider={authProvider} title="Swarmpit XPX">
    <Resource name="services" list={ServiceList} show={ServiceShow} icon={ViewListIcon} />
    <Resource name="stacks" list={StackList} icon={LayersIcon} />
    <Resource name="nodes" list={NodeList} show={NodeShow} icon={DnsIcon} />
    <Resource name="networks" list={NetworkList} icon={NetworkCheckIcon} />
    <Resource name="volumes" list={VolumeList} icon={StorageIcon} />
    <Resource name="secrets" list={SecretList} icon={LockIcon} />
    <Resource name="configs" list={ConfigList} icon={SettingsIcon} />
    <Resource name="tasks" list={TaskList} icon={AssignmentIcon} />
    <Resource name="audit" list={AuditList} icon={HistoryIcon} />
  </Admin>
);

export default App;
