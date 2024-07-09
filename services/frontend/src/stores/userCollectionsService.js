import axios from 'axios';

export async function fetchUserCollections(token) {
  try {
    const response = await axios.get('https://animeenigma.ru/api/animeCollections', {
      headers: {
        Authorization: `Bearer ${token}`
      }
    });
    return response.data;
  } catch (error) {
    console.error('Error fetching collections:', error.response.data);
    throw error;
  }
}

export async function createUserCollection(payload, token) {
  try {
    const response = await axios.post('https://animeenigma.ru/api/animeCollections', payload, {
      headers: {
        Authorization: `Bearer ${token}`
      }
    });
    return response.data;
  } catch (error) {
    console.error('Error creating collection:', error.response.data);
    throw error;
  }
}
