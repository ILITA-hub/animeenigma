export function getRandomNumber(min, max) {
    return Math.floor(Math.random() * (max - min) + min)
}

export function getPlayingOpemimg(openings, history) {
    while (true) {
        const index = getRandomNumber(0, openings.length - 1)
        const openingPlay = openings[index]
        if (!history.includes(openingPlay)) {
            return openingPlay
        }
    }
}

export function shuffle(arr){
	var j, temp;
	for(var i = arr.length - 1; i > 0; i--){
		j = Math.floor(Math.random()*(i + 1));
		temp = arr[j];
		arr[j] = arr[i];
		arr[i] = temp;
	}
	return arr;
}