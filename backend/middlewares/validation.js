
import Joi from 'joi';

function validator(joiSchema, reqParameter = 'body') {
    return function (req, res, next) {
        const { error } = joiSchema.validate(req[reqParameter]);

        if (!error) {
            return next();
        }

        return res.status(406).json({
            error
        });

    };
}

const exampleSchema = Joi.object({
    query: Joi.string().required(),
    limit: Joi.number().min(1).max(200).default(20),
    offset: Joi.number().default(0)
})

const roomsPost = Joi.object({
    name: Joi.string().required(),
    userId: Joi.string().required(),
});

export {
    validator,
    roomsPost
}
