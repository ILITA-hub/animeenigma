class Room {
    id: number;
    name: string;
    status: string;
    openingId: string;
    users: object;
    updatedAt: number;
    ownerId: number;
    historyAnime: Array<number>;
    rangeAnime: Array<number>;
}

// const Status = { START: 'start', PLAYING: 'playing', BREAK: 'break' };

export { Room }