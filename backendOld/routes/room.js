import { Room, Status } from '../resources/roomClass.js'
import { Router } from 'express'
import client from '../utils/caches.js'
import { generateRoomId } from '../utils/miscellaneous.js'
import { validator, roomsPost } from '../middlewares/validation.js'

const router = Router()

router.get('/getAll', async (req, res) => {
    res.send(await client.get("rooms"))
})

router.get('/:roomId', async (req, res) => {
    const roomId = req.params.roomId
    const result = await client.get(`room${roomId}`)
    if (!result) {
        res.sendStatus(404)
        return
    }
    res.send(result)
})

router.post('/',
    validator(roomsPost),
    async (req, res) => {

        const roomId = generateRoomId();
        const body = req.body
        const newRoom = new Room(roomId, body.name, Status.START, null, [], Number(new Date()), body.userId)
        await client.set(`room${roomId}`, newRoom)

        const allRooms = await client.get("rooms")
        if (!allRooms) {
            await client.set("rooms", [roomId])
        } else {
            await client.set("rooms", [...allRooms, roomId])
        }

        res.send(roomId)
    })

router.delete('/:roomId', async (req, res) => {
    const roomId = req.params.roomId

    const roomToDelete = await client.get(`room${roomId}`)
    if (!roomToDelete) {
        res.sendStatus(404)
        return
    }
    if (roomToDelete.ownerId != req.body.userId) {
        res.sendStatus(403)
        return
    }

    const allRooms = await client.get("rooms")
    if (!allRooms) {
        res.sendStatus(404)
        return
    }

    const newRooms = allRooms.filter(room => room != roomId)
    await client.set("rooms", newRooms)

    await client.del(`room${roomId}`)

    res.sendStatus(200)
})

router.put('/:roomId', async () => {
    const roomId = req.params.roomId
    const body = req.body

    const roomToUpdate = await client.get(`room${roomId}`)
    if (!roomToUpdate) {
        res.sendStatus(404)
        return
    }
    if (roomToUpdate.ownerId != body.userId) {
        res.sendStatus(403)
        return
    }


})

export default router