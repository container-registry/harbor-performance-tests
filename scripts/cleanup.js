// cleanup test data
import { Harbor } from 'k6/x/harbor'

import { getEnv } from './config.js'
import { fetchProjects } from './helpers.js'

export let options = {
    duration: '24h',
    vus: 1,
    iterations: 1,
};

const harbor = new Harbor({
    scheme: getEnv('HARBOR_SCHEME', 'https'),
    host: getEnv('HARBOR_HOST'),
    username: getEnv('HARBOR_USERNAME', 'admin'),
    password: getEnv('HARBOR_PASSWORD', 'Harbor12345'),
    insecure: true,
})

export default function () {
    // delete projects (force=true deletes repositories first)
    const projects = fetchProjects(harbor)
    for (let i = 0; i < projects.length; i++) {
        if (projects[i].name === 'library') {
            continue
        }

        try {
            harbor.deleteProject(projects[i].name, true)
            console.log(`deleted project ${projects[i].name}`)
        } catch (e) {
            console.error(`failed to delete project ${projects[i].name}, error: ${e.message}`)
        }
    }

    // delete test users
    const listResult = harbor.listUsers({ page: 1, pageSize: 100 })
    const users = listResult.users
    for (let i = 0; i < users.length; i++) {
        if (users[i].username === 'admin') {
            continue
        }
        try {
            harbor.deleteUser(users[i].userID)
            console.log(`deleted user ${users[i].username}`)
        } catch (e) {
            console.error(`failed to delete user ${users[i].username}, error: ${e.message}`)
        }
    }

    // clean up local temp files (content store)
    harbor.free()
}
